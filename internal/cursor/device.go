package cursor

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/denisbrodbeck/machineid"
)

// GetDeviceID handles logic related to GetDeviceID.
func GetDeviceID() (string, error) {
	deviceID, err := machineid.ProtectedID("cursor")
	if err != nil || strings.TrimSpace(deviceID) == "" {
		rawID, rawErr := machineid.ID()
		if rawErr != nil {
			if err != nil {
				return "", fmt.Errorf("failed to get device ID: %w", err)
			}
			return "", fmt.Errorf("failed to get device ID: %w", rawErr)
		}
		deviceID = rawID
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", errors.New("failed to get device ID: device ID is empty")
	}
	return deviceID, nil
}

// defaultDeviceMeta handles logic related to defaultDeviceMeta.
func defaultDeviceMeta() string {
	return fmt.Sprintf("%s / %s", displayOSName(runtime.GOOS), runtime.GOARCH)
}

// displayOSName handles logic related to displayOSName.
func displayOSName(goos string) string {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return goos
	}
}
