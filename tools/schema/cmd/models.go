package cmd

// DB mimics the general info needed for services used to define placeholders.
type DB struct {
	Host       string
	User       string
	Pass       string
	Database   string
	Driver     string
	DisableTLS bool
}

