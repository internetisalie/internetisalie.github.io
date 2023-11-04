package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/md"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/net/html"

	"github.com/gomarkdown/markdown/parser"
)

const templateDir = `.template`
const repositoryFile = `repository.json`
const indexFile = `index.html`
const readmeFile = `README.md`

// render instantiates a single template into the target directory
func render(name, match string) error {
	templateFile := filepath.Join(templateDir, match)

	var tpl *template.Template
	tpl, err := template.New(match).ParseFiles(templateFile)
	if err != nil {
		return err
	}

	output := filepath.Join(name, match)
	writer, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer writer.Close()

	return tpl.Execute(writer, name)
}

func instance(name string) error {
	if err := os.MkdirAll(name, 0755); err != nil {
		return err
	}

	dirFS := os.DirFS(templateDir)

	matches, err := fs.Glob(dirFS, "*")
	if err != nil {
		return err
	}

	for _, match := range matches {
		if err = render(name, match); err != nil {
			return err
		}
	}

	return nil
}

// index updates the root index.html link list
func index(name string) error {
	documentData, err := os.ReadFile(indexFile)
	if err != nil {
		return err
	}

	document, err := html.Parse(bytes.NewBuffer(documentData))
	if err != nil {
		return err
	}

	// find the unordered list
	elementPathParts := []string{"html", "body", "div", "ul"}
	element := document
	for len(elementPathParts) > 0 {
		child := element.FirstChild
		for child != nil {
			if child.Type == html.ElementNode && child.Data == elementPathParts[0] {
				element = child
				elementPathParts = elementPathParts[1:]
				break
			}
			child = child.NextSibling
		}
	}

	// create our item
	newChildren := []*html.Node{
		{
			Type: html.ElementNode,
			Data: "li",
			FirstChild: &html.Node{
				Type: html.ElementNode,
				Data: "a",
				FirstChild: &html.Node{
					Type: html.TextNode,
					Data: name,
				},
				Attr: []html.Attribute{{
					Key: "href",
					Val: "/" + name,
				}},
			},
		},
		{
			Type: html.TextNode,
			Data: "\n            ",
		},
	}

	// add our item
	child := element.FirstChild
	for child != nil {
		if child.Type == html.ElementNode && child.FirstChild.FirstChild.Data >= name {
			break
		}
		child = child.NextSibling
	}
	if child != nil {
		if child.FirstChild.FirstChild.Data == name {
			// don't duplicate
			return nil
		}
		for _, newChild := range newChildren {
			element.InsertBefore(newChild, child)
		}
	} else {
		for _, newChild := range newChildren {
			element.AppendChild(newChild)
		}
	}

	documentBuffer := new(bytes.Buffer)
	if err = html.Render(documentBuffer, document); err != nil {
		return err
	}

	return os.WriteFile(indexFile, documentBuffer.Bytes(), 0644)
}

// indexMarkdown updates the root index.md link list
func readme(name string) error {
	documentData, err := os.ReadFile(readmeFile)
	if err != nil {
		return err
	}

	// create Markdown parser
	p := parser.NewWithExtensions(parser.CommonExtensions)

	// create new list item
	newListItemText := fmt.Sprintf("- [%s](/%s)\n", name, name)
	newListItemList := p.Parse([]byte(newListItemText))
	newListItem := newListItemList.
		AsContainer().Children[0].
		AsContainer().Children[0]
	newListItem.(*ast.ListItem).ListFlags = 0

	// parse document
	p = parser.NewWithExtensions(parser.CommonExtensions)
	document := p.Parse(documentData)
	list := document.AsContainer().Children[1].AsContainer()
	entries := list.Children
	insertAt := len(entries)
	for i, entry := range entries {
		listItem := entry.AsContainer().Children[0]
		link := listItem.AsContainer().Children[1]
		linkText := link.AsContainer().Children[0]

		content := string(linkText.AsLeaf().Literal)
		if content == name {
			return nil
		} else if content > name {
			insertAt = i
			break
		}
	}

	if insertAt < len(entries) {
		beforeEntries, afterEntries := entries[:insertAt], entries[insertAt:]
		entries = slices.Clone(beforeEntries)
		entries = append(entries, newListItem)
		entries = append(entries, afterEntries...)
		if insertAt == 0 {
			newListItem.(*ast.ListItem).ListFlags = ast.ListItemBeginningOfList
			// clear beginning of list on previously-first item
			entries[1].(*ast.ListItem).ListFlags = 0
		}
	} else {
		newListItem.(*ast.ListItem).ListFlags = ast.ListItemEndOfList
		entries = append(entries, newListItem)
		if len(entries) > 1 {
			// clear end of list on previously-last item
			entries[len(entries)-2].(*ast.ListItem).ListFlags = 0
		}
	}

	list.Children = entries

	renderer := md.NewRenderer()
	documentData = markdown.Render(document, renderer)

	return os.WriteFile(readmeFile, documentData, 0644)
}

func record(name string) error {
	data, err := os.ReadFile(repositoryFile)
	if err != nil {
		return err
	}

	var repositories []string
	if err = json.Unmarshal(data, &repositories); err != nil {
		return err
	}

	for _, repository := range repositories {
		if repository == name {
			return nil
		}
	}

	repositories = append(repositories, name)
	slices.Sort(repositories)

	data, err = json.MarshalIndent(repositories, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(repositoryFile, data, 0644)
}

func main() {
	logger := log.New(os.Stdout, "repository: ", 0)
	if len(os.Args) == 1 {
		logger.Fatal("missing repository name argument")
	}

	name := os.Args[1]

	if err := instance(name); err != nil {
		logger.Fatal(err.Error())
	}

	if err := index(name); err != nil {
		logger.Fatal(err.Error())
	}

	if err := readme(name); err != nil {
		logger.Fatal(err.Error())
	}

	if err := record(name); err != nil {
		logger.Fatal(err.Error())
	}
}
