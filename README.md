# Radish

[![GoDoc](https://godoc.org/github.com/rotationalio/radish?status.svg)](https://godoc.org/github.com/rotationalio/radish)
[![Go Report Card](https://goreportcard.com/badge/github.com/rotationalio/radish)](https://goreportcard.com/report/github.com/rotationalio/radish)


NOTE: the task kind cannot be dynamic and must be encoded on a per-struct basis. This is because the worker instantiates a new zero-valued task and calls its Kind() method to get the task kind on registration. Dynamic task kind values will not be correctly assigned to a worker.
