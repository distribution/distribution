// registry-api-doctable-gen uses various descriptors within the registry code
// base to generate markdown tables for use in documentation. This is only
// meant to facilitate updates to documentation and not as an automated tool.
//
// For now, this only includes support for error codes:
//
// 	$ registry-api-doctable-gen errors
//
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/docker/docker-registry/api/v2"
)

func main() {

	if len(os.Args) < 2 {
		log.Fatalln("please specify a table to generate: (errors)")
	}

	switch os.Args[1] {
	case "errors":
		dumpErrors(os.Stdout)
	default:
		log.Fatalln("unknown descriptor table:", os.Args[1])
	}

}

func dumpErrors(wr io.Writer) {
	writer := tabwriter.NewWriter(os.Stdout, 8, 8, 0, '\t', 0)
	defer writer.Flush()

	fmt.Fprint(writer, "|")
	dtype := reflect.TypeOf(v2.ErrorDescriptor{})
	var fieldsPrinted int
	for i := 0; i < dtype.NumField(); i++ {
		field := dtype.Field(i)
		if field.Name == "Value" {
			continue
		}

		fmt.Fprint(writer, field.Name, "|")
		fieldsPrinted++
	}

	divider := strings.Repeat("-", 8)
	var parts []string
	for i := 0; i < fieldsPrinted; i++ {
		parts = append(parts, divider)
	}
	divider = strings.Join(parts, "|")

	fmt.Fprintln(writer, "\n"+divider)

	for _, descriptor := range v2.ErrorDescriptors {
		fmt.Fprint(writer, "|")

		v := reflect.ValueOf(descriptor)
		for i := 0; i < dtype.NumField(); i++ {
			value := v.Field(i).Interface()
			field := v.Type().Field(i)
			if field.Name == "Value" {
				continue
			} else if field.Name == "Description" {
				value = strings.Replace(value.(string), "\n", " ", -1)
			} else if field.Name == "Code" {
				value = fmt.Sprintf("`%s`", value)
			} else if field.Name == "HTTPStatusCodes" {
				if len(value.([]int)) > 0 {
					var codes []string
					for _, code := range value.([]int) {
						codes = append(codes, fmt.Sprint(code))
					}
					value = strings.Join(codes, ", ")
				} else {
					value = "Any"
				}

			}

			fmt.Fprint(writer, value, "|")
		}

		fmt.Fprint(writer, "\n")
	}
}
