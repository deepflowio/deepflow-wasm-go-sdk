package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"text/template"

	"google.golang.org/protobuf/proto"
	descriptor "google.golang.org/protobuf/types/descriptorpb"
	plugin "google.golang.org/protobuf/types/pluginpb"
)

// getGoPackage returns the file's go_package option.
// If it containts a semicolon, only the part before it is returned.
func getGoPackage(fd *descriptor.FileDescriptorProto) string {
	pkg := fd.GetOptions().GetGoPackage()
	if strings.Contains(pkg, ";") {
		parts := strings.Split(pkg, ";")
		if len(parts) > 2 {
			log.Fatalf(
				"protoc-gen-demo: go_package '%s' contains more than 1 ';'",
				pkg)
		}
		pkg = parts[1]
	}

	return pkg
}

// goPackageOption interprets the file's go_package option.
// If there is no go_package, it returns ("", "", false).
// If there's a simple name, it returns ("", pkg, true).
// If the option implies an import path, it returns (impPath, pkg, true).
func goPackageOption(d *descriptor.FileDescriptorProto) (impPath, pkg string, ok bool) {
	pkg = getGoPackage(d)
	if pkg == "" {
		return
	}
	ok = true
	// The presence of a slash implies there's an import path.
	slash := strings.LastIndex(pkg, "/")
	if slash < 0 {
		return
	}
	impPath, pkg = pkg, pkg[slash+1:]
	// A semicolon-delimited suffix overrides the package name.
	sc := strings.IndexByte(impPath, ';')
	if sc < 0 {
		return
	}
	impPath, pkg = impPath[:sc], impPath[sc+1:]
	return
}

// goFileName returns the output name for the generated Go file.
func goFileName(d *descriptor.FileDescriptorProto) string {
	name := *d.Name
	if ext := path.Ext(name); ext == ".proto" || ext == ".protodevel" {
		name = name[:len(name)-len(ext)]
	}
	name += ".demo.go"

	if pathsSourceRelative {
		return name
	}

	// Does the file have a "go_package" option?
	// If it does, it may override the filename.
	if impPath, _, ok := goPackageOption(d); ok && impPath != "" {
		// Replace the existing dirname with the declared import path.
		_, name = path.Split(name)
		name = path.Join(impPath, name)
		return name
	}

	return name
}

func splitTypePath(name string) []string {
	if len(name) == 0 {
		log.Fatal("Empty message type")
	}
	if name[0] != '.' {
		log.Fatalf("Expect message type name to start with '.', but it is '%s'", name)
	}
	return strings.Split(name[1:], ".")
}

func lookupFileDescriptor(name string) *descriptor.FileDescriptorProto {
	for _, fd := range request.GetProtoFile() {
		if fd.GetPackage() == name {
			return fd
		}
	}
	return nil
}

func lookupMessageType(name string) (*descriptor.FileDescriptorProto, *descriptor.DescriptorProto) {
	path := splitTypePath(name)

	pkgpath := path[:len(path)-1]

	var fd *descriptor.FileDescriptorProto
	for {
		pkgname := strings.Join(pkgpath, ".")
		fd = lookupFileDescriptor(pkgname)
		if fd != nil {
			break
		}
		if len(pkgpath) == 1 {
			log.Fatalf("Could not find the .proto file for package '%s' (from message %s)", pkgname, name)
		}
		pkgpath = pkgpath[:len(pkgpath)-1]
	}

	path = path[len(pkgpath):]

	var d *descriptor.DescriptorProto
	for _, mt := range fd.GetMessageType() {
		if mt.GetName() == path[0] {
			d = mt
			break
		}
	}
	if d == nil {
		log.Fatalf("No such type '%s' in package '%s'", path[0], strings.Join(pkgpath, "."))
	}
	for i, token := range path[1:] {
		var found bool
		for _, nd := range d.GetNestedType() {
			if nd.GetName() == token {
				d = nd
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("No such nested type '%s' in '%s.%s'",
				token, strings.Join(pkgpath, "."), strings.Join(path[:i+1], "."))
		}
	}
	return fd, d
}

func getGoType(pbType string) (string, string) {
	if !strings.Contains(pbType, ".") {
		return "", pbType
	}
	fd, _ := lookupMessageType(pbType)
	name := strings.TrimPrefix(pbType, "."+fd.GetPackage()+".")
	name = strings.Replace(name, ".", "_", -1)
	return getGoPackage(fd), name
}

func getPkgImportName(goPkg string) string {
	if goPkg == getGoPackage(currentFile) {
		return ""
	}
	replacer := strings.NewReplacer(".", "_", "/", "_", "-", "_")
	return replacer.Replace(goPkg)
}

// baseName returns the last path element of the name, with the last dotted suffix removed.
func baseName(name string) string {
	// First, find the last element
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	// Now drop the suffix
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[0:i]
	}
	return name
}

// goPackageName returns the Go package name to use in the
// generated Go file.  The result explicit reports whether the name
// came from an option go_package statement.  If explicit is false,
// the name was derived from the protocol buffer's package statement
// or the input file name.
func goPackageName(d *descriptor.FileDescriptorProto) (name string, explicit bool) {
	// Does the file have a "go_package" option?
	if _, pkg, ok := goPackageOption(d); ok {
		return pkg, true
	}

	// Does the file have a package clause?
	if pkg := d.GetPackage(); pkg != "" {
		return pkg, false
	}
	// Use the file base name.
	return baseName(d.GetName()), false
}

var pathsSourceRelative bool

var funcMap = template.FuncMap{
	"GoPackageName": func(fd *descriptor.FileDescriptorProto) string {
		p, _ := goPackageName(fd)
		return p
	},
	"GoType": func(pbType string) string {
		goPkg, goType := getGoType(pbType)
		if goPkg != "" {
			importName := getPkgImportName(goPkg)
			if importName != "" {
				goType = importName + "." + goType
			}
		}
		return goType
	},
}

var request plugin.CodeGeneratorRequest
var currentFile *descriptor.FileDescriptorProto

func main() {

	log.SetPrefix("protoc-gen-demo: ")
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("error: reading input: %v", err)
	}

	var response plugin.CodeGeneratorResponse
	if err := proto.Unmarshal(data, &request); err != nil {
		log.Fatalf("error: parsing input proto: %v", err)
	}

	if len(request.GetFileToGenerate()) == 0 {
		log.Fatal("error: no files to generate")
	}

	for _, param := range strings.Split(request.GetParameter(), ",") {
		var value string
		if i := strings.Index(param, "="); i >= 0 {
			value = param[i+1:]
			param = param[0:i]
		}
		switch param {
		case "":
			// Ignore.
		case "plugins":
			for _, plugin := range strings.Split(value, ";") {
				switch plugin {
				default:
					log.Fatalf("invalid plugin: %s", plugin)
				}
			}
		case "paths":
			if value == "source_relative" {
				pathsSourceRelative = true
			} else if value == "import" {
				pathsSourceRelative = false
			} else {
				log.Fatalf(`unknown path type %q: want "import" or "source_relative"`, value)
			}
		}
	}

	tmpl, err := template.New(".").Funcs(funcMap).Parse(tFile)
	if err != nil {
		log.Fatal(err)
	}

	for _, name := range request.GetFileToGenerate() {
		var fd *descriptor.FileDescriptorProto
		for _, fd = range request.GetProtoFile() {
			if name == fd.GetName() {
				break
			}
		}
		if fd == nil {
			log.Fatalf("could not find the .proto file for %s", name)
		}

		currentFile = fd

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, fd); err != nil {
			log.Fatal(err)
		}

		currentFile = nil

		response.File = append(response.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(goFileName(fd)),
			Content: proto.String(buf.String()),
		})
	}

	if data, err = proto.Marshal(&response); err != nil {
		log.Fatalf("error: failed to marshal output proto: %v", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		log.Fatalf("error: failed to write output proto: %v", err)
	}
}
