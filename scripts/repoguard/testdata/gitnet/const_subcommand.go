package fake

func bad(c *Client) {
	const verb = "pull"
	c.cmd(verb, "origin", "main")
}
