package rollover

import "html/template"

var Mappings = map[string]template.HTML{
	"empty": `"mappings": {}`,
	"geely": template.HTML(`
		"mappings": {
			"logs": {
				"_source": {
					"enabled": true
				},
				"_all": {
					"enabled": false
				},
				"properties": {
					"msgid": {
						"type": "long"
					},
					"level": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"tag": {
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
				"_all": {
					"enabled": false
				},
				"properties": {
					"msgid": {
						"type": "long"
					},
					"level": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"tag": {
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
					"_all": {
						"enabled": false
					},
					"properties": {
						"msgid": {
							"type": "long"
						},
						"level": {
							"type": "keyword",
							"index": "not_analyzed",
							"doc_values": true
						},
						"tag": {
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
				"_all": {
					"enabled": false
				},
				"properties": {
					"msgid": {
						"type": "long"
					},
					"level": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"tag": {
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
					"_all": {
						"enabled": false
					},
					"properties": {
						"msgid": {
							"type": "long"
						},
						"level": {
							"type": "keyword",
							"index": "not_analyzed",
							"doc_values": true
						},
						"tag": {
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
	"emqtt": template.HTML(`
			"mappings": {
				"logs": {
					"_source": {
						"enabled": true
					},
					"_all": {
						"enabled": false
					},
					"properties": {
						"msgid": {
							"type": "long"
						},
						"datasource": {
							"type": "keyword",
							"index": "not_analyzed",
							"doc_values": true
						},
						"tag": {
							"type": "keyword",
							"index": "not_analyzed",
							"doc_values": true
						},
						"hostname": {
							"type": "keyword",
							"index": "not_analyzed",
							"doc_values": true
						},
						"priority": {
							"type": "short"
						},
						"facility": {
							"type": "short"
						},
						"severity": {
							"type": "short"
						},
						"content": {
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
				"_all": {
					"enabled": false
				},
				"properties": {
					"msgid": {
						"type": "long"
					},
					"level": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"tag": {
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
	"wuling": template.HTML(`
	    "mappings": {
			"logs": {
				"_source": {
					"enabled": true
				},
				"_all": {
					"enabled": false
				},
				"properties": {
					"msgid": {
						"type": "long"
					},
					"tag": {
						"type": "keyword",
						"index": "not_analyzed",
						"doc_values": true
					},
					"vin": {
						"type": "keyword"
					},
					"rowkey": {
						"type": "keyword"
					},
					"location": {
						"type": "geo_point"
					}
				}
			}
		}`),
}
