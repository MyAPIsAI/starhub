package controller

import (
	"reflect"
	"testing"

	"starhub/relay/channel"
)

func TestPlazaProtocols(t *testing.T) {
	tests := []struct {
		name  string
		types []int
		tags  []string
		want  []string
	}{
		{"chat conversion", []int{channel.ChannelTypeGemini}, []string{"text"}, []string{"OpenAI", "Anthropic"}},
		{"vision chat", []int{channel.ChannelTypeClaude}, []string{"text", "vision"}, []string{"OpenAI", "Anthropic"}},
		{"image generation", []int{channel.ChannelTypeOpenAI}, []string{"image"}, []string{"OpenAI"}},
		{"embeddings", []int{channel.ChannelTypeOpenAI}, []string{"embedding"}, []string{"OpenAI"}},
		{"audio", []int{channel.ChannelTypeOpenAI}, []string{"audio"}, []string{"OpenAI"}},
		{"unsupported media channel", []int{channel.ChannelTypeClaude}, []string{"video"}, []string{}},
		{"unknown channel", []int{999}, []string{"text"}, []string{}},
		{"no channel", nil, []string{"text"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plazaProtocols(tt.types, tt.tags); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("protocols = %v, want %v", got, tt.want)
			}
		})
	}
}
