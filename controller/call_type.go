package controller

import "strings"

func classifyCallType(path string) string {
	switch {
	case strings.Contains(path, "embedding"):
		return "embeddings"
	case strings.Contains(path, "rerank"):
		return "rerank"
	case strings.Contains(path, "image"):
		return "images"
	case strings.Contains(path, "audio") || strings.Contains(path, "speech"):
		return "audio"
	case strings.Contains(path, "video"):
		return "video"
	case strings.Contains(path, "responses"):
		return "responses"
	default:
		return "chat"
	}
}
