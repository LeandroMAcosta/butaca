package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/LeandroMAcosta/butaca/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show and change settings"}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		RunE: func(*cobra.Command, []string) error {
			out, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Printf("# %s\n%s", cfg.Path(), out)
			if problems := cfg.Validate(); len(problems) > 0 {
				fmt.Println("\n# problems:")
				for _, p := range problems {
					fmt.Println("#  -", p)
				}
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Write a config file with the defaults",
		RunE: func(*cobra.Command, []string) error {
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Println("wrote", cfg.Path())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a value, e.g. paths.movies ~/Movies",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := setValue(cfg, args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("%s = %s\n", args[0], args[1])
			return nil
		},
	})

	return cmd
}

// setValue walks the struct by yaml tag so keys read the same as the file.
func setValue(cfg *config.Config, key, value string) error {
	parts := strings.Split(key, ".")
	v := reflect.ValueOf(cfg).Elem()

	for i, part := range parts {
		if v.Kind() != reflect.Struct {
			return fmt.Errorf("%s is not a section", strings.Join(parts[:i], "."))
		}
		field, ok := fieldByYAML(v, part)
		if !ok {
			return fmt.Errorf("unknown key %q", key)
		}
		if i == len(parts)-1 {
			return assign(field, value)
		}
		v = field
	}
	return fmt.Errorf("unknown key %q", key)
}

func fieldByYAML(v reflect.Value, name string) (reflect.Value, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if tag == name {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func assign(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int64:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%q is not a number", value)
		}
		field.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%q is not true or false", value)
		}
		field.SetBool(b)
	case reflect.Slice:
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		field.Set(reflect.ValueOf(parts))
	default:
		return fmt.Errorf("cannot set a %s", field.Kind())
	}
	return nil
}
