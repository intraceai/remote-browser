package browser

import (
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/playwright-community/playwright-go"
)

type Browser struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	context playwright.BrowserContext
	page    playwright.Page
	mu      sync.Mutex
}

type CaptureResult struct {
	Screenshot string `json:"screenshot"`
	DOM        string `json:"dom"`
	FinalURL   string `json:"final_url"`
	Viewport   struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"viewport"`
	UserAgent string `json:"user_agent"`
}

func New() (*Browser, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("could not start playwright: %w", err)
	}

	execPath := "/usr/bin/chromium-browser"

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless:       playwright.Bool(false),
		ExecutablePath: playwright.String(execPath),
		Args: []string{
			"--no-sandbox",
			"--disable-setuid-sandbox",
			"--disable-dev-shm-usage",
			"--disable-gpu",
		},
	})
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("could not launch browser: %w", err)
	}

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1280,
			Height: 720,
		},
	})
	if err != nil {
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("could not create context: %w", err)
	}

	page, err := context.NewPage()
	if err != nil {
		context.Close()
		browser.Close()
		pw.Stop()
		return nil, fmt.Errorf("could not create page: %w", err)
	}

	return &Browser{
		pw:      pw,
		browser: browser,
		context: context,
		page:    page,
	}, nil
}

func (b *Browser) OpenURL(url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, err := b.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	})
	return err
}

func (b *Browser) Capture() (*CaptureResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	screenshot, err := b.page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
		Type:     playwright.ScreenshotTypePng,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to take screenshot: %w", err)
	}

	dom, err := b.page.Content()
	if err != nil {
		return nil, fmt.Errorf("failed to get content: %w", err)
	}

	viewport := b.page.ViewportSize()
	width, height := 1280, 720
	if viewport != nil {
		width = viewport.Width
		height = viewport.Height
	}

	userAgent, _ := b.page.Evaluate("navigator.userAgent")
	userAgentStr := ""
	if ua, ok := userAgent.(string); ok {
		userAgentStr = ua
	}

	result := &CaptureResult{
		Screenshot: base64.StdEncoding.EncodeToString(screenshot),
		DOM:        dom,
		FinalURL:   b.page.URL(),
		UserAgent:  userAgentStr,
	}
	result.Viewport.Width = width
	result.Viewport.Height = height

	return result, nil
}

func (b *Browser) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.context != nil {
		b.context.Close()
	}
	if b.browser != nil {
		b.browser.Close()
	}
	if b.pw != nil {
		b.pw.Stop()
	}
	return nil
}
