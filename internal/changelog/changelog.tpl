{{range .Commits}}- [{{ .Message }}]({{ $.Config.VCSURL }}/{{ .Hash }})
{{end}}