// Parses and provides configurations for the Wehe server.
package config

import (
    "fmt"

    "gopkg.in/ini.v1"
)

// Configurations for the Wehe server
// configs are read in from a .ini config file
type Config struct {
    TestsDir string
    PortNumbersFile string
}

// Creates a new Config object
// configPath: path to the .ini config file
// Returns a configuration struct or an error
func New(configPath *string) (Config, error) {
    config := Config{}

    configFile, err := ini.Load(*configPath)
    if err != nil {
        return config, err
    }
    defaultSection := configFile.Section("")

    config.TestsDir, err = getString(defaultSection, "tests_dir")
    if err != nil {
        return config, err
    }

    config.PortNumbersFile, err = getString(defaultSection, "port_numbers_file")
    if err != nil {
        return config, err
    }

    return config, nil
}

// Gets a string from the config file.
// section: the section of the ini file that contains the key
// keyStr: the key
// Returns the value of the key or an error
func getString(section *ini.Section, keyStr string) (string, error) {
    key, err := section.GetKey(keyStr)
    if err != nil {
        return "", err
    }
    val := key.String()
    if val == "" {
        return "", fmt.Errorf("No value read from %s key", keyStr)
    }
    return val, nil
}

// Gets a log level from the config file.
// section: the section of the ini file that contains the key
// keyStr: the key
// Returns the integer value of the log level or an error
func getLogLevel(section *ini.Section, keyStr string) (int, error) {
    val, err := getString(section, keyStr)
    if err != nil {
        return -1, err
    }

    switch val {
    case "wtf":
        return 1, nil
    case "error":
        return 2, nil
    case "warn":
        return 3, nil
    case "info":
        return 4, nil
    case "debug":
        return 5, nil
    default:
        return -1, fmt.Errorf("%s is not a log level. Choose from ui, wtf, error, warn, info, or debug.", val)
    }
}

// Gets an integer from the config file.
// section: the section of the ini file that contains the key
// keyStr: the key
// low: the lower bounds (inclusive) that the value should not go below
// high: the upper bounds (inclusive) that the value should not go above
// Returns the value or an error
func getInt(section *ini.Section, keyStr string, low int, high int) (int, error) {
    key, err := section.GetKey(keyStr)
    if err != nil {
        return -1, err
    }
    val, err := key.Int()
    if err != nil {
        return -1, fmt.Errorf("%s in %s key", err, keyStr)
    }
    if val < low || val > high {
        return -1, fmt.Errorf("%d is not a valid number for %s. Must be between %d and %d inclusive.", val, keyStr, low, high)
    }
    return val, nil
}

// Gets a boolean from the config file.
// section: the section of the ini file that contains the key
// keyStr: the key
//Returns the value or an error
func getBool(section *ini.Section, keyStr string) (bool, error) {
    key, err := section.GetKey(keyStr)
    if err != nil {
        return false, err
    }
    val, err := key.Bool()
    if err != nil {
        return false, fmt.Errorf("%s in %s key", err, keyStr)
    }
    return val, nil
}
