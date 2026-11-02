package fake

func bad(c *Client, b string) {
	c.cmd("push", "origin", b)
}
