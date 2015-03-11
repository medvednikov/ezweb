package gomvc

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"regexp"
	"strings"
)

// Custom html/template functions
var defaultFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"mul": func(a, b int) int { return a * b },
	"inc": func(n int) int { return n + 1 },
	"tojson": func(i interface{}) template.JS {
		out, _ := json.Marshal(i)
		res := template.JS(out)
		return res
	},
	"js": func(file string) template.HTML {
		if strings.Index(file, "//") == -1 {
			file = "/js/" + file
		}
		file += fmt.Sprintf("?%d", TimeStamp)
		return template.HTML("<script src='" + file + "'></script>")
	},
	"css": func(file string) template.HTML {
		if strings.Index(file, "//") == -1 {
			file = "/css/" + file
		}
		file += fmt.Sprintf("?%d", TimeStamp)
		return template.HTML("<link href='" + file + "' rel='stylesheet'>")
	},
	"staticcss": func(file string) template.HTML {
		if strings.Index(file, "//") == -1 {
			file = "/css/" + file
		}
		return template.HTML("<link href='" + file + "' rel='stylesheet'>")
	},
	"staticjs": func(file string) template.HTML {
		if strings.Index(file, "//") == -1 {
			file = "/js/" + file
		}
		return template.HTML("<script src='" + file + "'></script>")
	},
}

// readTemplate reads a template file on dev, or an asset file on production
// and returns its contents
func readTemplate(path string) string {

	if !Debug && AssetFunc != nil {
		b, err := AssetFunc(path)
		if err != nil {
			log.Println("Asset error", err)
			return ""
		}
		return convertTemplate(b)
	}

	b, err := ioutil.ReadFile("v/" + path)
	if err != nil {
		log.Println("Reading template error", err)
		return ""
	}

	return convertTemplate(b)
}

// convertTemplate applies custom structures and functions and converts a
// custom template to Go's HTML template
func convertTemplate(b []byte) string {
	s := string(b)

	r := regexp.MustCompile(`@\*(.*?)\*@`)
	s = r.ReplaceAllString(s, "")

	r = regexp.MustCompile(`@t ([a-zA-Z_0-9]+)`)
	s = r.ReplaceAllString(s, `{{template "$1"}}`)

	r = regexp.MustCompile(`@\.`)
	s = r.ReplaceAllString(s, "{{.}}")

	r = regexp.MustCompile("@(if|else|end|range|template|define)(.*?)\n")
	s = r.ReplaceAllString(s, "{{ $1 $2 }}\n")

	r = regexp.MustCompile("@([A-Z][a-zA-Z\\.]+)")
	s = r.ReplaceAllString(s, "{{.$1}}")

	r = regexp.MustCompile(`@([a-z][a-zA-Z\\.]+( "[^"]+")*)`)
	s = r.ReplaceAllString(s, "{{ $1 }}")

	r = regexp.MustCompile("%([a-zA-Z_0-9]+)")
	s = r.ReplaceAllString(s, `{{ T "$1" }}`)

	return s
}
