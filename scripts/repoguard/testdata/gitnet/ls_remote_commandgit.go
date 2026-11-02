package fake

func bad(ctx context.Context, env []string) {
	_ = CommandGit(ctx, env, "ls-remote", "--refs", "origin")
}
