package fake

func bad(c *Client) {
	c.cmd("fetch", "origin", "main")
}
