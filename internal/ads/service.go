package ads

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"cursor/internal/netproxy"

	"gopkg.in/yaml.v3"
)

const (
	defaultFetchTimeout = 45 * time.Second
	maxZipBytes         = 50 << 20
	maxAssetBytes       = 20 << 20
	noAdStatusCode      = http.StatusNotFound
)

type MetricsProvider func(context.Context) (MetricsSnapshot, error)
type ProviderCountProvider func(context.Context) (int, error)
type DeviceIDProvider func() (string, error)

type Options struct {
	StoreRoot     string
	HTTPClient    *http.Client
	AppVersion    string
	AssetBaseURL  string
	DeviceID      DeviceIDProvider
	Metrics       MetricsProvider
	ProviderCount ProviderCountProvider
}

type Service struct {
	storeRoot     string
	httpClient    *http.Client
	appVersion    string
	assetBaseURL  string
	deviceID      DeviceIDProvider
	metrics       MetricsProvider
	providerCount ProviderCountProvider

	assetBaseURLMu sync.RWMutex
	refreshMu      sync.Mutex
}

type parsedPackage struct {
	hash       string
	rawConfig  []byte
	configJSON string
	assets     []parsedAsset
}

type parsedAsset struct {
	path        string
	contentType string
	data        []byte
}

type adPackageFile struct {
	Hash        string                `json:"hash"`
	ConfigJSON  string                `json:"config_json"`
	Assets      map[string]adAssetRef `json:"assets"`
	FetchedAt   time.Time             `json:"fetched_at"`
	ActivatedAt time.Time             `json:"activated_at"`
}

type adAssetRef struct {
	File        string `json:"file"`
	ContentType string `json:"content_type"`
}

type adConfigYAML struct {
	Enabled *bool               `yaml:"enabled"`
	Window  WindowConfig        `yaml:"window"`
	Home    HomePlacementConfig `yaml:"home"`
}

type packageState int

const (
	packageMissing packageState = iota
	packageValid
	packageCorrupt
)

type packageInspection struct {
	state packageState
	pkg   adPackageFile
}

func NewService(options Options) *Service {
	client := options.HTTPClient
	if client == nil {
		client = netproxy.NewHTTPClient(defaultFetchTimeout)
	}
	return &Service{
		storeRoot:     strings.TrimSpace(options.StoreRoot),
		httpClient:    client,
		appVersion:    strings.TrimSpace(options.AppVersion),
		assetBaseURL:  normalizeAssetBaseURL(options.AssetBaseURL),
		deviceID:      options.DeviceID,
		metrics:       options.Metrics,
		providerCount: options.ProviderCount,
	}
}

func (service *Service) SetAssetBaseURL(rawURL string) bool {
	if service == nil {
		return false
	}
	next := normalizeAssetBaseURL(rawURL)
	service.assetBaseURLMu.Lock()
	defer service.assetBaseURLMu.Unlock()
	if service.assetBaseURL == next {
		return false
	}
	service.assetBaseURL = next
	return true
}

func (service *Service) currentAssetBaseURL() string {
	if service == nil {
		return ""
	}
	service.assetBaseURLMu.RLock()
	defer service.assetBaseURLMu.RUnlock()
	return service.assetBaseURL
}

func normalizeAssetBaseURL(rawURL string) string {
	return strings.TrimRight(strings.TrimSpace(rawURL), "/")
}

func (service *Service) Refresh(parent context.Context) (Runtime, bool, error) {
	if service == nil {
		return Runtime{}, false, fmt.Errorf("ad service is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	service.refreshMu.Lock()
	defer service.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, defaultFetchTimeout)
	defer cancel()
	result, err := service.FetchOnce(ctx)
	if err != nil {
		return Runtime{}, false, err
	}
	runtimeState, err := service.GetRuntime(context.Background())
	if err != nil {
		return Runtime{}, result.Changed, err
	}
	return runtimeState, result.Changed, nil
}

func (service *Service) FetchOnce(ctx context.Context) (FetchResult, error) {
	// 已完全禁用远程广告与 Telemetry 抓取
	return FetchResult{Changed: false}, nil
}

func (service *Service) fetchSlotOnce(ctx context.Context, slot Slot) (FetchResult, error) {
	return FetchResult{Changed: false}, nil
}

func (service *Service) CurrentHash(ctx context.Context) (string, error) {
	return service.currentHash(ctx, normalizeSlotID(""))
}

func (service *Service) currentHash(ctx context.Context, slotID string) (string, error) {
	inspection := service.inspectSlotPackage(ctx, slotID)
	if inspection.state != packageValid {
		return "", nil
	}
	return strings.TrimSpace(inspection.pkg.Hash), nil
}

func (service *Service) GetRuntime(ctx context.Context) (Runtime, error) {
	runtimeState := Runtime{
		AssetBaseURL: service.currentAssetBaseURL(),
	}
	for _, slot := range Slots {
		slotRuntime, err := service.getSlotRuntime(ctx, normalizeSlotID(slot.ID))
		if err != nil {
			return runtimeState, err
		}
		runtimeState.Slots = append(runtimeState.Slots, slotRuntime)
	}
	if len(runtimeState.Slots) > 0 {
		first := runtimeState.Slots[0]
		runtimeState.Available = first.Available
		runtimeState.Enabled = first.Enabled
		runtimeState.PackageHash = first.PackageHash
		runtimeState.AssetBaseURL = first.AssetBaseURL
		runtimeState.IndexURL = first.IndexURL
		runtimeState.Window = first.Window
		runtimeState.Home = first.Home
	}
	return runtimeState, nil
}

func (service *Service) getSlotRuntime(ctx context.Context, slotID string) (SlotRuntime, error) {
	slotID = normalizeSlotID(slotID)
	runtimeState := SlotRuntime{
		ID:           slotID,
		AssetBaseURL: service.assetURL(slotID, ""),
		IndexURL:     service.assetURL(slotID, "index.html"),
	}
	inspection := service.inspectSlotPackage(ctx, slotID)
	if inspection.state != packageValid {
		return runtimeState, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(inspection.pkg.ConfigJSON), &cfg); err != nil {
		return runtimeState, nil
	}
	runtimeState.Available = true
	runtimeState.Enabled = cfg.Enabled
	runtimeState.PackageHash = strings.TrimSpace(inspection.pkg.Hash)
	runtimeState.Window = cfg.Window
	runtimeState.Home = cfg.Home
	return runtimeState, nil
}

func (service *Service) LoadAsset(ctx context.Context, rawPath string) (parsedAsset, string, bool, error) {
	// Remote ads and their cached JavaScript are disabled. Do not serve legacy
	// cache contents left by an older version of the application.
	return parsedAsset{}, "", false, nil
}

func (service *Service) applyReportHeaders(ctx context.Context, request *http.Request, currentHash string) {
	// 已禁用所有 Telemetry / 设备 ID / Token 统计数据上报 Headers
	return
}

func (service *Service) currentPackage(ctx context.Context, slotID string) (adPackageFile, bool, error) {
	_ = ctx
	inspection := service.inspectSlotPackage(ctx, slotID)
	switch inspection.state {
	case packageMissing:
		return adPackageFile{}, false, nil
	case packageValid:
		return inspection.pkg, true, nil
	default:
		return adPackageFile{}, false, fmt.Errorf("广告缓存已损坏")
	}
}

func (service *Service) inspectSlotPackage(ctx context.Context, slotID string) packageInspection {
	_ = ctx
	body, err := os.ReadFile(service.slotPackagePath(slotID))
	if err != nil {
		if os.IsNotExist(err) {
			return packageInspection{state: packageMissing}
		}
		return packageInspection{state: packageCorrupt}
	}
	var item adPackageFile
	if err := json.Unmarshal(body, &item); err != nil {
		return packageInspection{state: packageCorrupt}
	}
	if item.Assets == nil {
		item.Assets = make(map[string]adAssetRef)
	}
	if !validSlotStorageHash(slotID, item.Hash) {
		return packageInspection{state: packageCorrupt}
	}
	var cfg Config
	if err := json.Unmarshal([]byte(item.ConfigJSON), &cfg); err != nil {
		return packageInspection{state: packageCorrupt}
	}
	if cfg.Window.Width <= 0 || cfg.Window.Height <= 0 {
		return packageInspection{state: packageCorrupt}
	}
	if _, ok := item.Assets["index.html"]; !ok {
		return packageInspection{state: packageCorrupt}
	}
	for assetPath, ref := range item.Assets {
		if _, err := normalizeArchivePath(assetPath); err != nil {
			return packageInspection{state: packageCorrupt}
		}
		if _, ok, err := service.readAssetFile(slotID, assetPath, ref); err != nil || !ok {
			return packageInspection{state: packageCorrupt}
		}
	}
	return packageInspection{state: packageValid, pkg: item}
}

func (service *Service) readAssetFile(slotID string, assetPath string, ref adAssetRef) ([]byte, bool, error) {
	fileName := strings.TrimSpace(ref.File)
	if !isSafeAssetFileName(fileName) {
		return nil, false, nil
	}
	filePath := filepath.Join(service.slotAssetsDir(slotID), fileName)
	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !assetFileMatchesData(assetPath, fileName, data) {
		return nil, false, nil
	}
	return data, true, nil
}

func isSafeAssetFileName(fileName string) bool {
	if fileName == "" || fileName == "." || fileName == ".." {
		return false
	}
	if filepath.IsAbs(fileName) || path.IsAbs(fileName) {
		return false
	}
	if strings.Contains(fileName, "/") || strings.Contains(fileName, "\\") || strings.Contains(fileName, "..") {
		return false
	}
	return true
}

func assetFileMatchesData(assetPath string, fileName string, data []byte) bool {
	sum := sha256.Sum256(data)
	expectedPrefix := hex.EncodeToString(sum[:])
	expectedExtension := strings.TrimPrefix(strings.ToLower(path.Ext(assetPath)), ".")
	if expectedExtension == "" {
		return strings.EqualFold(fileName, expectedPrefix)
	}
	actualPrefix, extension, ok := strings.Cut(fileName, ".")
	if !ok || !strings.EqualFold(actualPrefix, expectedPrefix) {
		return false
	}
	return strings.EqualFold(extension, expectedExtension)
}

func validSlotStorageHash(slotID string, hash string) bool {
	hash = strings.TrimSpace(hash)
	expectedPrefix := normalizeSlotID(slotID) + ":"
	if !strings.HasPrefix(hash, expectedPrefix) {
		return false
	}
	value := strings.TrimPrefix(hash, expectedPrefix)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (service *Service) replacePackage(ctx context.Context, slotID string, parsed parsedPackage) error {
	_ = ctx
	now := time.Now().UTC()
	slotDir := service.slotDir(slotID)
	tempDir := slotDir + ".tmp-" + strconv.FormatInt(now.UnixNano(), 10)
	if err := os.MkdirAll(filepath.Join(tempDir, "assets"), 0o755); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()
	assets := make(map[string]adAssetRef, len(parsed.assets))
	for _, asset := range parsed.assets {
		fileName := adAssetFileName(asset)
		if err := os.WriteFile(filepath.Join(tempDir, "assets", fileName), asset.data, 0o644); err != nil {
			return err
		}
		assets[asset.path] = adAssetRef{
			File:        fileName,
			ContentType: asset.contentType,
		}
	}
	pkg := adPackageFile{
		Hash:        parsed.hash,
		ConfigJSON:  parsed.configJSON,
		Assets:      assets,
		FetchedAt:   now,
		ActivatedAt: now,
	}
	if err := writeJSONFile(filepath.Join(tempDir, "package.json"), pkg); err != nil {
		return err
	}
	backupDir := slotDir + ".old-" + strconv.FormatInt(now.UnixNano(), 10)
	hadExisting := false
	if _, err := os.Stat(slotDir); err == nil {
		hadExisting = true
		if err := os.Rename(slotDir, backupDir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tempDir, slotDir); err != nil {
		if hadExisting {
			_ = os.Rename(backupDir, slotDir)
		}
		return err
	}
	cleanup = false
	if hadExisting {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func (service *Service) clearPackage(ctx context.Context, slotID string) (bool, error) {
	_ = ctx
	slotDir := service.slotDir(slotID)
	if _, err := os.Stat(slotDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, os.RemoveAll(slotDir)
}

func (service *Service) assetURL(slotID string, assetPath string) string {
	base := strings.TrimRight(service.currentAssetBaseURL(), "/")
	if base == "" {
		return ""
	}
	slotID = normalizeSlotID(slotID)
	cleaned := strings.TrimLeft(strings.TrimSpace(assetPath), "/")
	if cleaned == "" {
		return base + "/" + slotID
	}
	return base + "/" + slotID + "/" + cleaned
}

func (service *Service) slotDir(slotID string) string {
	root := strings.TrimSpace(service.storeRoot)
	if root == "" {
		root = "ads"
	}
	return filepath.Join(root, normalizeSlotID(slotID))
}

func (service *Service) slotAssetsDir(slotID string) string {
	return filepath.Join(service.slotDir(slotID), "assets")
}

func (service *Service) slotPackagePath(slotID string) string {
	return filepath.Join(service.slotDir(slotID), "package.json")
}

func adAssetFileName(asset parsedAsset) string {
	sum := sha256.Sum256(asset.data)
	extension := strings.ToLower(path.Ext(asset.path))
	return hex.EncodeToString(sum[:]) + extension
}

func writeJSONFile(filePath string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, append(data, '\n'), 0o644)
}

func parsePackage(hash string, data []byte) (parsedPackage, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return parsedPackage{}, fmt.Errorf("解析广告压缩包失败: %w", err)
	}
	seen := map[string]struct{}{}
	var rawConfig []byte
	var assets []parsedAsset
	for _, file := range reader.File {
		if file == nil || file.FileInfo().IsDir() {
			continue
		}
		cleanPath, err := normalizeArchivePath(file.Name)
		if err != nil {
			return parsedPackage{}, err
		}
		if _, exists := seen[cleanPath]; exists {
			return parsedPackage{}, fmt.Errorf("广告压缩包存在重复文件: %s", cleanPath)
		}
		seen[cleanPath] = struct{}{}
		payload, err := readZipFile(file)
		if err != nil {
			return parsedPackage{}, err
		}
		if cleanPath == "config.yaml" {
			rawConfig = payload
		}
		assets = append(assets, parsedAsset{
			path:        cleanPath,
			contentType: detectContentType(cleanPath, payload),
			data:        payload,
		})
	}
	if len(rawConfig) == 0 {
		return parsedPackage{}, fmt.Errorf("广告压缩包缺少根目录 config.yaml")
	}
	if _, ok := seen["index.html"]; !ok {
		return parsedPackage{}, fmt.Errorf("广告压缩包缺少根目录 index.html")
	}
	cfg, err := parseConfig(rawConfig)
	if err != nil {
		return parsedPackage{}, err
	}
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return parsedPackage{}, err
	}
	return parsedPackage{
		hash:       strings.TrimSpace(hash),
		rawConfig:  rawConfig,
		configJSON: string(configJSON),
		assets:     assets,
	}, nil
}

func parseConfig(data []byte) (Config, error) {
	var raw adConfigYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("解析广告 config.yaml 失败: %w", err)
	}
	if raw.Enabled == nil {
		return Config{}, fmt.Errorf("广告 config.yaml 缺少 enabled")
	}
	cfg := Config{
		Enabled: *raw.Enabled,
		Window: WindowConfig{
			Width:  raw.Window.Width,
			Height: raw.Window.Height,
		},
		Home: HomePlacementConfig{
			Title:    strings.TrimSpace(raw.Home.Title),
			Subtitle: strings.TrimSpace(raw.Home.Subtitle),
		},
	}
	if cfg.Window.Width <= 0 || cfg.Window.Height <= 0 {
		return Config{}, fmt.Errorf("广告 window.width/window.height 必须为正整数")
	}
	return cfg, nil
}

func normalizeArchivePath(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("广告压缩包存在空文件名")
	}
	if strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("广告压缩包存在非法路径: %s", raw)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("广告压缩包存在路径穿越: %s", raw)
	}
	return cleaned, nil
}

func normalizeRequestAssetPath(raw string) (string, string, error) {
	pathValue := strings.TrimSpace(raw)
	pathValue = strings.TrimPrefix(pathValue, RoutePrefix)
	pathValue = strings.TrimLeft(pathValue, "/")
	slotID := normalizeSlotID("")
	if head, tail, ok := strings.Cut(pathValue, "/"); ok && isKnownSlotID(head) {
		slotID = normalizeSlotID(head)
		pathValue = tail
	} else if isKnownSlotID(pathValue) {
		slotID = normalizeSlotID(pathValue)
		pathValue = ""
	}
	if pathValue == "" {
		pathValue = "index.html"
	}
	assetPath, err := normalizeArchivePath(pathValue)
	return slotID, assetPath, err
}

func normalizeSlotID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "1"
	}
	return value
}

func isKnownSlotID(value string) bool {
	value = normalizeSlotID(value)
	for _, slot := range Slots {
		if normalizeSlotID(slot.ID) == value {
			return true
		}
	}
	return false
}

func slotStorageHash(slotID string, hash string) string {
	return normalizeSlotID(slotID) + ":" + strings.TrimSpace(hash)
}

func stripSlotStorageHash(hash string) string {
	_, value, ok := strings.Cut(strings.TrimSpace(hash), ":")
	if !ok {
		return strings.TrimSpace(hash)
	}
	return strings.TrimSpace(value)
}

func readZipFile(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > maxAssetBytes {
		return nil, fmt.Errorf("广告资源过大: %s", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readLimited(reader, maxAssetBytes, "广告资源 "+file.Name)
}

func readLimited(reader io.Reader, limit int64, label string) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s超过大小限制", strings.TrimSpace(label))
	}
	return data, nil
}

func detectContentType(assetPath string, data []byte) string {
	extension := strings.ToLower(path.Ext(assetPath))
	if extension != "" {
		if value := mime.TypeByExtension(extension); strings.TrimSpace(value) != "" {
			return value
		}
	}
	switch extension {
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	}
	if len(data) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(data)
}

func headerValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
