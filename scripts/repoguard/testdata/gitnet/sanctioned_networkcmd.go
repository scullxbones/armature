package fake

func ok(c *Client) {
	_, cancel := c.networkCmd("fetch", "origin", "main")
	defer cancel()
	_ = c.cmd("status", "--porcelain")
}
