package utils

import (
	"fmt"
	"net/http"
	"testing"
)

func TestRequestJSON(t *testing.T) {
	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	RequestJSON("POST", "http://httpbin.org/post", &data, &resp)
	if fmt.Sprintf("%v", resp) != want {
		t.Errorf("got: %v", resp)
	}
}
func TestRequestJSONWithClient(t *testing.T) {
	data := RequestData{
		Data: map[string]string{
			"hello": "world",
		},
	}
	var resp struct {
		JSON map[string]string `json:"json"`
	}
	want := "{map[hello:world]}"
	httpClient := &http.Client{}
	RequestJSONWithClient(httpClient, "POST", "http://httpbin.org/post", &data, &resp)
	if fmt.Sprintf("%v", resp) != want {
		t.Errorf("got: %v", resp)
	}
}
