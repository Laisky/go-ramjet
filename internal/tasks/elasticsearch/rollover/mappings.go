package rollover

import (
	"html/template"

	"github.com/Laisky/go-utils"
)

func getESMapping(name string) template.HTML {
	return template.HTML(utils.Settings.GetString("tasks.elasticsearch.mappings." + name))
}
