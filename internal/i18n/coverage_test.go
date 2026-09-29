package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// portugueseMarker reconhece acentos e palavras frequentes do português.
var portugueseMarker = regexp.MustCompile(`(?i)[ãõçáéíóúâêô]|\b(não|foi|está|de|da|do|para|com|uma|um|os|as|ao|pelo|pela|sem|já|ainda)\b`)

var formatVerb = regexp.MustCompile(`%[-+# 0]*(\[\d+\])?(\d+|\*)?(\.(\d+|\*))?[a-zA-Z]`)

// portugueseAllowedInEnglish lista textos que continuam em português de
// propósito: marcadores comparados internamente e nomes que não são exibidos.
var portugueseAllowedInEnglish = map[string]string{
	"já possui uma operação em andamento": "marcador comparado com a resposta, junto com o equivalente em inglês",
	"transição":                           "marcador comparado com a resposta, junto com o equivalente em inglês",
	"Em transição":                        "estado interno traduzido na saída por Text",
	"— Como usar os comandos":             "constante; cada uso passa por i18n.Choose",
	"memória":                             "nome de métrica do AMP usado na busca, não é exibido",
}

type sourceLiteral struct {
	position token.Position
	value    string
	call     *ast.CallExpr
}

// walkSourceLiterals percorre os literais de texto do código de produção,
// fora deste pacote, informando a chamada que os contém diretamente.
func walkSourceLiterals(t *testing.T, visit func(sourceLiteral)) {
	t.Helper()

	fset := token.NewFileSet()
	err := filepath.Walk(filepath.Join("..", ".."), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "i18n" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		var calls []*ast.CallExpr
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ImportSpec:
				return false
			case *ast.CallExpr:
				calls = append(calls, typed)
			case *ast.BasicLit:
				if typed.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(typed.Value)
				if err != nil {
					return true
				}
				var parent *ast.CallExpr
				for index := len(calls) - 1; index >= 0; index-- {
					for _, argument := range calls[index].Args {
						if argument == typed {
							parent = calls[index]
						}
					}
					if parent != nil {
						break
					}
				}
				visit(sourceLiteral{position: fset.Position(typed.Pos()), value: value, call: parent})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isPair(call *ast.CallExpr) bool {
	if call == nil || len(call.Args) != 2 {
		return false
	}
	for _, argument := range call.Args {
		if literal, ok := argument.(*ast.BasicLit); !ok || literal.Kind != token.STRING {
			return false
		}
	}
	return true
}

func isChoose(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name, ok := selector.X.(*ast.Ident)
	return ok && name.Name == "i18n" && selector.Sel.Name == "Choose"
}

// isChooseAlias reconhece o apelido local "l := i18n.Choose" usado na
// montagem dos comandos e do guia.
func isChooseAlias(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	name, ok := call.Fun.(*ast.Ident)
	return ok && name.Name == "l"
}

// Uma tradução que troque, perca ou reordene um verbo de formatação quebra
// a mensagem só em um dos idiomas.
func TestChoosePairsKeepFormatVerbs(t *testing.T) {
	pairs := 0
	walkSourceLiterals(t, func(literal sourceLiteral) {
		if !(isChoose(literal.call) || isChooseAlias(literal.call)) || !isPair(literal.call) || literal.call.Args[0] != nodeAt(literal) {
			return
		}
		english, _ := strconv.Unquote(literal.call.Args[1].(*ast.BasicLit).Value)
		pairs++
		portugueseVerbs := strings.Join(formatVerb.FindAllString(strings.ReplaceAll(literal.value, "%%", ""), -1), " ")
		englishVerbs := strings.Join(formatVerb.FindAllString(strings.ReplaceAll(english, "%%", ""), -1), " ")
		if portugueseVerbs != englishVerbs {
			t.Errorf("%s: verbos diferentes: pt=[%s] en=[%s]", literal.position, portugueseVerbs, englishVerbs)
		}
	})
	if pairs < 100 {
		t.Fatalf("poucos pares i18n.Choose encontrados (%d); a varredura do código falhou", pairs)
	}
}

// Toda frase em português do código precisa de um par em inglês ou de uma
// entrada no catálogo de Text; senão ela sai em português em en-US.
func TestNoPortugueseLeftInEnglish(t *testing.T) {
	SetDefault(EnglishUS)
	defer SetDefault(PortugueseBrazil)

	walkSourceLiterals(t, func(literal sourceLiteral) {
		if len(literal.value) < 8 || !portugueseMarker.MatchString(literal.value) {
			return
		}
		if isChoose(literal.call) || isChooseAlias(literal.call) || (isPair(literal.call) && hasEnglishArgument(literal.call)) {
			return
		}
		if _, allowed := portugueseAllowedInEnglish[literal.value]; allowed {
			return
		}
		if translated := Text(literal.value); portugueseMarker.MatchString(translated) {
			t.Errorf("%s: texto em português sem tradução: %q (em inglês fica %q)", literal.position, literal.value, translated)
		}
	})
}

func hasEnglishArgument(call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		value, _ := strconv.Unquote(argument.(*ast.BasicLit).Value)
		if !portugueseMarker.MatchString(value) {
			return true
		}
	}
	return false
}

// nodeAt devolve o primeiro argumento quando ele é o literal visitado, para
// que cada par seja conferido uma única vez.
func nodeAt(literal sourceLiteral) ast.Expr {
	first, ok := literal.call.Args[0].(*ast.BasicLit)
	if !ok {
		return nil
	}
	value, _ := strconv.Unquote(first.Value)
	if value != literal.value {
		return nil
	}
	return first
}
