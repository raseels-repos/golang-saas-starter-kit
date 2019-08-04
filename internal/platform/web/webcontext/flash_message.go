package webcontext

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"html/template"
)

type FlashType string

var (
	FlashType_Success FlashType = "success"
	FlashType_Info    FlashType = "info"
	FlashType_Warning FlashType = "warning"
	FlashType_Error   FlashType = "danger"
)

type FlashMsg struct {
	Type    FlashType `json:"type"`
	Title   string    `json:"title"`
	Text    string    `json:"text"`
	Items   []string  `json:"items"`
	Details string    `json:"details"`
}

type FlashMsgResponse struct {
	Type    FlashType       `json:"type"`
	Title   template.HTML   `json:"title"`
	Text    template.HTML   `json:"text"`
	Items   []template.HTML `json:"items"`
	Details template.HTML   `json:"details"`
}

func (r FlashMsg) Response(ctx context.Context) FlashMsgResponse {
	var items []template.HTML
	for _, i := range r.Items {
		items = append(items, template.HTML(i))
	}

	return FlashMsgResponse{
		Type:    r.Type,
		Title:   template.HTML(r.Title),
		Text:    template.HTML(r.Text),
		Items:   items,
		Details: template.HTML(r.Details),
	}
}

func init() {
	gob.Register(&FlashMsg{})
}

// SessionAddFlash loads the session from context that is provided by the session middleware and
// adds the message to the session. The renderer should save the session before writing the response
// to the client or save be directly invoked.
func SessionAddFlash(ctx context.Context, msg FlashMsg) {
	dat, _ := json.Marshal(msg.Response(ctx))
	ContextSession(ctx).AddFlash(dat)
}

// SessionFlashSuccess add a message with type Success.
func SessionFlashSuccess(ctx context.Context, title, text string, items ...string) {
	sessionFlashType(ctx, FlashType_Success, title, text, items...)
}

// SessionFlashInfo add a message with type Info.
func SessionFlashInfo(ctx context.Context, title, text string, items ...string) {
	sessionFlashType(ctx, FlashType_Info, title, text, items...)
}

// SessionFlashWarning add a message with type Warning.
func SessionFlashWarning(ctx context.Context, title, text string, items ...string) {
	sessionFlashType(ctx, FlashType_Warning, title, text, items...)
}

// SessionFlashError add a message with type Error.
func SessionFlashError(ctx context.Context, title, text string, items ...string) {
	sessionFlashType(ctx, FlashType_Error, title, text, items...)
}

// sessionFlashType adds a flash message with the specified type.
func sessionFlashType(ctx context.Context, flashType FlashType, title, text string, items ...string) {
	msg := FlashMsg{
		Type:  flashType,
		Title: title,
		Text:  text,
		Items: items,
	}
	SessionAddFlash(ctx, msg)
}
