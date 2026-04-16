package main

import "github.com/hungnm98/seshat-cli/testdata/go_sample/internal/order"

func main() {
	service := order.Service{}
	service.Create()
}
