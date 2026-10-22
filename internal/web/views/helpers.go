package views

import (
	"github.com/a-h/templ"

	"readmecheck/components/icon"
)

func featureIcon(name string) templ.Component {
	return icon.Icon(name)(icon.Props{Class: "size-4"})
}
