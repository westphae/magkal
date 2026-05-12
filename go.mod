module github.com/westphae/magkal

go 1.19

require (
	github.com/gorilla/websocket v1.5.3
	github.com/kidoman/embd v0.0.0-20170508013040-d3d8c0c5c68d
	github.com/westphae/goflying v0.6.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/golang/glog v1.2.5 // indirect

// Restate goflying's embd replace here: kidoman/embd panics on modern
// Raspberry Pi OS kernel strings (6.12.62+rpt-rpi-v8), and Go modules
// ignores replace directives that come from dependencies.
replace github.com/kidoman/embd => github.com/westphae/embd v0.1.0
