module github.com/westphae/magkal

go 1.22

require (
	github.com/gorilla/websocket v1.5.3
	github.com/westphae/goflying v0.6.1-0.20260513200734-e5018303242c
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/westphae/go-iio v0.2.0 // indirect

// Restate goflying's embd replace here: kidoman/embd panics on modern
// Raspberry Pi OS kernel strings (6.12.62+rpt-rpi-v8), and Go modules
// ignores replace directives that come from dependencies.
replace github.com/kidoman/embd => github.com/westphae/embd v0.1.0
