package adapters

func (c *Client) BundleCreateAdvertised() {
	bare := "/tmp/bare.git"
	spec := "HEAD:refs/heads/main"
	// gitnetallow: bundle-create-local-bare-push local bare path under armature-tmp-work, not a named remote
	_ = c.cmd("push", bare, spec)
}
