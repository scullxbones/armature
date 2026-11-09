package adapters_test

import "io"

func swallowErr(err error) { _ = err }

func swallowClose(c io.Closer) { swallowErr(c.Close()) }
