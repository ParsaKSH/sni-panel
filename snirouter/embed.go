package main

import _ "embed"

//go:embed web/index.html
var indexHTML []byte

//go:embed web/login.html
var loginHTML []byte
