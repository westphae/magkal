module github.com/westphae/magkal

go 1.19

require (
	github.com/gorilla/websocket v1.5.3
	github.com/kidoman/embd v0.0.0-20170508013040-d3d8c0c5c68d
	github.com/westphae/goflying v0.0.0-20200404223852-7c269d245cc5
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/golang/glog v1.2.5 // indirect

// Use the local goflying checkout so the cmd/websim Actual path picks up the
// AK09916 magnetometer support added to the icm20948 driver.
replace github.com/westphae/goflying => ../goflying

// kidoman/embd panics on modern Raspberry Pi OS kernel strings like
// "6.12.62+rpt-rpi-v8" (parseVersion can't handle the "+rpt" patch suffix).
// Replace directives in deps are ignored by Go modules, so the main module
// (this one) must declare the override even though goflying has the same one.
replace github.com/kidoman/embd => ../embd-fork
