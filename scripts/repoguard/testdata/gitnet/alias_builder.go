package fake

func bad(c *Client) {
	run := c.cmd
	run("clone", "https://example.invalid/repo.git", "dst")
}
