package rollover

import (
	"github.com/Laisky/go-utils"
	"html/template"
)

func getESMapping(name string) template.HTML {
	return template.HTML(utils.Settings.GetString("tasks.elasticsearch.mappings." + name))
}
