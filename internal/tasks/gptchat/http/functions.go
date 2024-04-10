package http

import "github.com/Laisky/errors/v2"

func Call(name, args string) (string, error) {
	switch name {
	case "how-to-subscribe":
		return howToSubscribe(args)
	default:
		return "", errors.Errorf("unknown function %q", name)
	}
}

func ToolsRequest() []OpenaiChatReqTool {
	return []OpenaiChatReqTool{
		{
			Type: "function",
			Function: OpenaiChatReqToolFunction{
				Name:        "how-to-subscribe",
				Description: "this function can answer how to subscribe",
			},
			// Parameters: ,
		},
	}
}

func howToSubscribe(_ string) (string, error) {
	return "yehoo", nil
}
