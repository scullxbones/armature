package fake

func bad(c *Client) {
	verb := "fetch"
	c.cmd(verb, "origin", "main")
}
