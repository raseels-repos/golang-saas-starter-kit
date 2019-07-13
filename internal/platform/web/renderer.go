package web

import (
	"context"
	"net/http"
)

type Renderer interface {
	Render(ctx context.Context, w http.ResponseWriter, req *http.Request, templateLayoutName, templateContentName, contentType string, statusCode int, data map[string]interface{}) error
	Error(ctx context.Context, w http.ResponseWriter, req *http.Request, statusCode int, er error) error
	Static(rootDir, prefix string) Handler
}
