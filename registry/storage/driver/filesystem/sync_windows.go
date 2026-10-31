package filesystem

// Windows does not support syncing a directory opened by os.Open.
func syncDir(string) error {
	return nil
}
