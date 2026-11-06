package main

import (
	"context"
	"log"

	"github.com/mistquay/fluentlauncher/pkg/pluginsdk"
)

type echoPlugin struct{}

func (echoPlugin) Query(ctx context.Context, query pluginsdk.Query) ([]pluginsdk.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text := query.Text
	if text == "" {
		text = "输入 echo 后跟文本"
	}
	return []pluginsdk.Result{{
		ID:          "echo",
		Title:       text,
		Subtitle:    "官方示例 · Enter 复制文本",
		Actions:     []pluginsdk.Action{{ID: "copy", Title: "复制文本", Kind: "copyText", Value: query.Text}},
		PreviewText: query.Text,
	}}, nil
}

func (echoPlugin) Execute(context.Context, string, string) error { return nil }

func main() {
	if err := pluginsdk.Serve("official.echo", echoPlugin{}); err != nil {
		log.Fatal(err)
	}
}
