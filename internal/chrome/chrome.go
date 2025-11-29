package chrome

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type Chrome struct {
	ctx       context.Context
	cancel    context.CancelFunc
	allocCtx  context.Context
	allocCancel context.CancelFunc
	mu        sync.Mutex

	// Frame callback
	onFrame func(data []byte)
}

type CaptureResult struct {
	Screenshot []byte
	DOM        string
	FinalURL   string
	Width      int
	Height     int
}

func New(onFrame func(data []byte)) (*Chrome, error) {
	chromePath := os.Getenv("CHROME_PATH")
	if chromePath == "" {
		chromePath = "/usr/bin/chromium"
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.Flag("renderer-process-limit", "1"),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.WindowSize(1280, 720),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Start browser
	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	c := &Chrome{
		ctx:         ctx,
		cancel:      cancel,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		onFrame:     onFrame,
	}

	return c, nil
}

func (c *Chrome) StartScreencast(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Set up screencast listener
	chromedp.ListenTarget(c.ctx, func(ev interface{}) {
		if frame, ok := ev.(*page.EventScreencastFrame); ok {
			if c.onFrame != nil {
				data, err := base64.StdEncoding.DecodeString(frame.Data)
				if err == nil {
					c.onFrame(data)
				}
			}
			// Acknowledge the frame
			go func() {
				chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
					return page.ScreencastFrameAck(frame.SessionID).Do(ctx)
				}))
			}()
		}
	})

	// Start screencast - JPEG format, quality 60, ~10 fps max
	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.StartScreencast().
			WithFormat(page.ScreencastFormatJpeg).
			WithQuality(60).
			WithMaxWidth(1280).
			WithMaxHeight(720).
			WithEveryNthFrame(6). // Skip more frames for lower fps
			Do(ctx)
	}))
}

func (c *Chrome) StopScreencast(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.StopScreencast().Do(ctx)
	}))
}

func (c *Chrome) Navigate(url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.Navigate(url))
}

func (c *Chrome) Capture() (*CaptureResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var screenshot []byte
	var html string
	var url string

	err := chromedp.Run(c.ctx,
		chromedp.FullScreenshot(&screenshot, 100),
		chromedp.OuterHTML("html", &html),
		chromedp.Location(&url),
	)
	if err != nil {
		return nil, err
	}

	return &CaptureResult{
		Screenshot: screenshot,
		DOM:        html,
		FinalURL:   url,
		Width:      1280,
		Height:     720,
	}, nil
}

func (c *Chrome) MouseMove(x, y float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
	}))
}

func (c *Chrome) MouseClick(x, y float64, button string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var btn input.MouseButton
	switch button {
	case "right":
		btn = input.Right
	case "middle":
		btn = input.Middle
	default:
		btn = input.Left
	}

	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := input.DispatchMouseEvent(input.MousePressed, x, y).
			WithButton(btn).
			WithClickCount(1).
			Do(ctx); err != nil {
			return err
		}
		return input.DispatchMouseEvent(input.MouseReleased, x, y).
			WithButton(btn).
			WithClickCount(1).
			Do(ctx)
	}))
}

func (c *Chrome) KeyPress(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.KeyEvent(key))
}

func (c *Chrome) TypeText(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.InsertText(text).Do(ctx)
	}))
}

func (c *Chrome) Scroll(x, y, deltaX, deltaY float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return chromedp.Run(c.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseWheel, x, y).
			WithDeltaX(deltaX).
			WithDeltaY(deltaY).
			Do(ctx)
	}))
}

func (c *Chrome) Close() error {
	c.cancel()
	c.allocCancel()
	return nil
}
