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
	"cp": template.HTML(`
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
					"message": {
						"type": "string"
					},
					"datasource": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					}
				}
			}
		}`),
	"gateway": template.HTML(`
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
						"app": {
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
						"line": {
							"type": "short"
						},
						"message": {
							"type": "string"
						}
					}
				}
			}`),
	"spring": template.HTML(`
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
					"app": {
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
					"line": {
						"type": "short"
					},
					"message": {
						"type": "string"
					},
					"datasource": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					}
				}
			}
		}`),
	"connector": template.HTML(`
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
						"app": {
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
						"line": {
							"type": "short"
						},
						"message": {
							"type": "string"
						}
					}
				}
			}`),
	"spark": template.HTML(`
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
					"app_info": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"message": {
						"type": "string"
					}
				}
			}
		}`),
}
