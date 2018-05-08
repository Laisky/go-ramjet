package rollover

import "html/template"

var Mappings = map[string]template.HTML{
	"geely": template.HTML(`
		"mappings": {
			"logs": {
				"_source": {
					"enabled": true
				},
				"properties": {
					"level": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"class": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"thread": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"project": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"line": {
						"type": "integer"
					},
					"message": {
						"type": "string"
					}
				}
			}
		}`),
}
