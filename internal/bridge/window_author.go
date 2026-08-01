package bridge

import "github.com/pkg/browser"

const footerAuthorHomeURL = "https://space.bilibili.com/311706663/upload/video"

var footerAuthorInfo = FooterAuthorInfo{
	ButtonText:        "Author leookun",
	DialogTitle:       "Author Message",
	DialogContent:     "This software is free software. If you were charged, you were likely scammed.\nWelcome to visit the author homepage https://space.bilibili.com/311706663/upload/video\nto view updates, tips, and future content.",
	DialogConfirmText: "Visit Homepage",
	DialogCancelText:  "Close",
}

// FooterAuthorInfo defines the display info of the author entry at the bottom of the home page.
type FooterAuthorInfo struct {
	ButtonText        string `json:"buttonText"`
	DialogTitle       string `json:"dialogTitle"`
	DialogContent     string `json:"dialogContent"`
	DialogConfirmText string `json:"dialogConfirmText"`
	DialogCancelText  string `json:"dialogCancelText"`
}

// GetFooterAuthorInfo returns the display info of the author entry at the bottom of the home page.
func (s *WindowService) GetFooterAuthorInfo() FooterAuthorInfo {
	return footerAuthorInfo
}

// OpenFooterAuthorHome opens the author's homepage.
func (s *WindowService) OpenFooterAuthorHome() error {
	return browser.OpenURL(footerAuthorHomeURL)
}
