package handler

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
)

type Entity struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Line   int      `json:"line"`
	Params []string `json:"params,omitempty"`
}

type Import struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
}

type Snap struct {
	Language string   `json:"language"`
	Entities []Entity `json:"entities"`
	Imports  []Import `json:"imports"`
	Exports  []string `json:"exports"`
}

type request struct {
	Code     string `json:"code"`
	Filename string `json:"filename"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "use POST"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // حد أقصى 2MB
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing code"})
		return
	}
	if req.Filename == "" {
		req.Filename = "input.go"
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, req.Filename, req.Code, parser.ParseComments)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s := Snap{Language: "go", Entities: []Entity{}, Imports: []Import{}, Exports: []string{}}

	for _, d := range file.Decls {
		switch x := d.(type) {
		case *ast.GenDecl:
			for _, sp := range x.Specs {
				t, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				kind := "type"
				switch t.Type.(type) {
				case *ast.StructType:
					kind = "struct"
				case *ast.InterfaceType:
					kind = "interface"
				}
				s.Entities = append(s.Entities, Entity{Kind: kind, Name: t.Name.Name, Line: fset.Position(t.Pos()).Line})
				if ast.IsExported(t.Name.Name) {
					s.Exports = append(s.Exports, t.Name.Name)
				}
			}
		case *ast.FuncDecl:
			en := Entity{Kind: "function", Name: x.Name.Name, Line: fset.Position(x.Pos()).Line}
			if x.Recv != nil {
				en.Kind = "method"
			}
			if x.Type.Params != nil {
				for _, p := range x.Type.Params.List {
					typ := types.ExprString(p.Type)
					if len(p.Names) == 0 {
						en.Params = append(en.Params, typ)
						continue
					}
					for _, n := range p.Names {
						en.Params = append(en.Params, n.Name+" "+typ)
					}
				}
			}
			s.Entities = append(s.Entities, en)
			if x.Recv == nil && ast.IsExported(x.Name.Name) {
				s.Exports = append(s.Exports, x.Name.Name)
			}
		}
	}

	for _, im := range file.Imports {
		s.Imports = append(s.Imports, Import{Source: im.Path.Value, Line: fset.Position(im.Pos()).Line})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"language": "go",
		"parser":   "go/parser",
		"strength": "real-ast",
		"snapshot": s,
	})
}
