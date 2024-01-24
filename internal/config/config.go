// Parses and provides configurations for the Wehe server.
package config

// Configurations for the Wehe server
// configs are read in from a .ini config file
type Config struct {

}

// Creates a new Config object
// configPath: path to the .ini config file
// Returns a configuration struct or an error
func New(configPath *string) (Config, error) {
    config := Config{}

    return config, nil
}
