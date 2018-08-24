package keyword_test

import (
	"testing"

	"github.com/Laisky/go-chaining"
	utils "github.com/Laisky/go-utils"

	"github.com/Laisky/go-ramjet/tasks/keyword"
)

var (
	analyser *keyword.Analyser
)

func TestAnalyser(t *testing.T) {
	words := analyser.Cut2Words("<p>南京市长江大桥</p>", 1, 10)
	for _, w := range words {
		if w == "南京市" {
			return
		}
	}
	t.Log(words)
	t.Error("analyser cut word error")
}

func TestDiscardRegex(t *testing.T) {
	words := []string{
		"p",
		"<",
		"ddwqwqd",
		">",
		"we",
	}
	ret, err := keyword.FilterDiscardWords(chaining.New(words, nil))
	if err != nil || ret.([]string)[0] != "ddwqwqd" {
		t.Error("analyser filter error")
	}
}

func TestDiscardFmt(t *testing.T) {
	cnt := `<div><a class="ddd" target="xxx"> d33kok </a></div>`
	ret, err := keyword.FilterFmt(chaining.New(cnt, nil))
	if err != nil {
		t.Errorf("FilterFmt error, got %+v", err)
	}
	if ret.(string) != "d33kok" {
		t.Errorf("FilterFmt error, expect d33kok, got %v", ret.(string))
	}
}

func init() {
	utils.SetupLogger("debug")
	analyser = keyword.NewAnalyser()
}
