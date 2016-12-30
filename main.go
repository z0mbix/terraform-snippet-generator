package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// An argument has several options
type Option struct {
	ForceNew bool
	Optional bool
	Computed bool
}

// Map of all arguments for this resource
type Arguments map[string]Option

// A Resource has arguments
type Resource struct {
	Name      string
	Arguments Arguments
}

// Generates terraform snippets for an editor from the terraform source code
func main() {
	// CLI flags
	sourcePath := flag.String("source", "", "Path to the Terraform source code")
	provider := flag.String("provider", "", "Provider to generate snippets for")
	editor := flag.String("editor", "vim", "Editor to generate snippets for")
	flag.Parse()

	providerPath := *sourcePath + "/builtin/providers/" + *provider

	resources := getResources(providerPath)
	for _, r := range resources {
		resourcePath := providerPath + "/" + r
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, resourcePath, nil, 0)
		if err != nil {
			fmt.Println(err)
			return
		}

		s, _ := parseSource(f, stripFileExtension(r))
		generateTerraformResource(s, *editor)
	}
}

// Convert resource_provider_name to resourceProviderName
func underscoreToCamel(s string) string {
	var result string
	words := strings.Split(s, "_")

	for i, word := range words {
		if len(word) > 0 && i > 0 {
			w := []rune(word)
			w[0] = unicode.ToUpper(w[0])
			result += string(w)
		} else {
			result += word
		}
	}
	return result
}

// Returns the function name to find in the AST
func getFuncName(p string) string {
	return underscoreToCamel(stripFileExtension(path.Base(p)))
}

// Returns string with p removed
func stripPrefix(s string, p string) string {
	return strings.Replace(s, p, "", -1)
}

// Remove file extension from filename
func stripFileExtension(f string) string {
	return f[0 : len(f)-len(filepath.Ext(f))]
}

// Returns a slice of all .go files in provider directory
func getProviderFiles(path string) ([]string, error) {
	// TODO: Support data sources
	files, _ := filepath.Glob(path + "/resource_*.go")
	return files, nil
}

// Returns a slice of all resources in provider directory
func getResources(p string) []string {
	r := make([]string, 0)
	files, err := getProviderFiles(p)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		r = append(r, filepath.Base(f))
	}

	return r
}

// Increments counter in template
func incrementCounter(i int) int {
	return i + 1
}

func generateTerraformResource(r Resource, e string) {
	f := template.FuncMap{"increment": incrementCounter}
	t := template.New(e + ".tmpl").Funcs(f)
	t, err := t.ParseFiles("tmpl/" + e + ".tmpl")
	if err != nil {
		log.Fatal(err)
	}
	// TODO: Output to stdout or write to a file, or options for both?
	err = t.Execute(os.Stdout, r)
	if err != nil {
		log.Fatal(err)
	}
}

func getArgumentName(kv *ast.KeyValueExpr) string {
	s, _ := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
	return s
}

func getArgumentOptions(unary *ast.UnaryExpr) (*Option, error) {
	schemaFound := false
	a := &Option{}
	x := unary.X.(*ast.CompositeLit)
	for _, v := range x.Elts {
		item := v.(*ast.KeyValueExpr)
		// spew.Dump(item)
		switch item.Value.(type) {
		case *ast.SelectorExpr:
			if item.Value.(*ast.SelectorExpr).X.(*ast.Ident).Name == "schema" {
				schemaFound = true
			}
		case *ast.Ident:
			k := item.Key.(*ast.Ident).Name
			v, _ := strconv.ParseBool(item.Value.(*ast.Ident).Name)

			if k == "Optional" {
				a.Optional = v
			}
			if k == "Computed" {
				a.Computed = v
			}
			if k == "ForceNew" {
				a.ForceNew = v
			}
		}
	}
	if schemaFound == false {
		return nil, fmt.Errorf("schema not found")
	}

	return a, nil
}

func parseSource(f *ast.File, resource string) (Resource, error) {
	d := f.Decls
	arguments := make(Arguments)
	r := getFuncName(resource)

	for _, v := range d {
		switch v.(type) {
		case *ast.FuncDecl:
			f := v.(*ast.FuncDecl)
			for i := range f.Body.List {
				if f.Name.Name == r {
					listItem := f.Body.List[i].(*ast.ReturnStmt)
					for _, v := range listItem.Results {

						u := v.(*ast.UnaryExpr)
						cl := u.X.(*ast.CompositeLit)

						for _, field := range cl.Elts {

							ff := field.(*ast.KeyValueExpr)

							if ff.Key.(*ast.Ident).Name == "Schema" {
								cl := ff.Value.(*ast.CompositeLit)

								for _, vvv := range cl.Elts {
									myKV := vvv.(*ast.KeyValueExpr)
									name := getArgumentName(myKV)
									var r *Option
									var err error
									if name == "tags" {
										// tags are nested, need to look into this the type seems
										// to be ast.CallExpr what ever that is, so we ignore for now
										continue
									} else {
										r, err = getArgumentOptions(myKV.Value.(*ast.UnaryExpr))
										if err != nil {
											fmt.Println(err)
										}
									}
									// Ignore computed arguments
									if !r.Computed {
										arguments[name] = *r
									}
								}
							}
						}
					}
				}

			}
		}
	}

	rName := stripPrefix(resource, "resource_")
	return Resource{rName, arguments}, nil
}
