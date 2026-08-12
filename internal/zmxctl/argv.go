package zmxctl

func EnsureArgv(name string) []string {
	return []string{"attach", name}
}

func AttachArgv(name string) []string {
	return []string{"attach", name}
}

func SendArgv(name string) []string {
	return []string{"send", name}
}

func HistoryArgv(name string, vt bool) []string {
	args := []string{"history", name}
	if vt {
		args = append(args, "--vt")
	}
	return args
}

func ListArgv() []string {
	return []string{"list"}
}

func KillArgv(name string) []string {
	return []string{"kill", name}
}
