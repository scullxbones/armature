package fake

func bad(c *Client, b string) {
	args := []string{"push", "origin"}
	args = append(args, b)
	c.cmd(args...)
}
