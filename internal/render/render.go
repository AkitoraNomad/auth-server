package render

import (
	"html/template"
	"net/http"
)

var tpl *template.Template

func LoadTemplates(pattern string) error {
	parsed, err := template.New("").ParseGlob(pattern)
	if err != nil {
		return err
	}
	tpl = parsed
	return nil
}

func Html(w http.ResponseWriter, tplName string, code int, data any) error {
	w.WriteHeader(code)
	return tpl.ExecuteTemplate(w, tplName, data)
}
