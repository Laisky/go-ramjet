package rollover

import (
	"html/template"

	gconfig "github.com/Laisky/go-config"
)

func getESMapping(name string) template.HTML {
	return template.HTML(gconfig.Shared.GetString("tasks.elasticsearch.mappings." + name))
}
