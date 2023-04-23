package errcheckanalyzer

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

type Pass struct {
	// отобразим здесь только важные поля
	Fset         *token.FileSet // информация о позиции токенов
	Files        []*ast.File    // AST для каждого файла
	OtherFiles   []string       // имена файлов не на Go в пакете
	IgnoredFiles []string       // имена игнорируемых исходных файлов в пакете
	Pkg          *types.Package // информация о типах пакета
	TypesInfo    *types.Info    // информация о типах в AST
}

var ErrCheckAnalyzer = &analysis.Analyzer{
	Name: "errcheck",
	Doc:  "check for unchecked errors",
	Run:  run,
}

var FuncMainEnd int

/*
	func run(pass *analysis.Pass) (interface{}, error) {
	    // реализация будет ниже
	    return nil, nil
	}
*/
var errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

func isErrorType(t types.Type) bool {
	return types.Implements(t, errorType)
}

// resultErrors возвращает булев массив со значениями true,
// если тип i-го возвращаемого значения соответствует ошибке.
func resultErrors(pass *analysis.Pass, call *ast.CallExpr) []bool {
	switch t := pass.TypesInfo.Types[call].Type.(type) {
	case *types.Named: // возвращается значение
		return []bool{isErrorType(t)}
	case *types.Pointer: // возвращается указатель
		return []bool{isErrorType(t)}
	case *types.Tuple: // возвращается несколько значений
		s := make([]bool, t.Len())
		for i := 0; i < t.Len(); i++ {
			switch mt := t.At(i).Type().(type) {
			case *types.Named:
				s[i] = isErrorType(mt)
			case *types.Pointer:
				s[i] = isErrorType(mt)
			}
		}
		return s
	}
	return []bool{false}
}

// isReturnError возвращает true, если среди возвращаемых значений есть ошибка.
func isReturnError(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, isError := range resultErrors(pass, call) {
		if isError {
			return true
		}
	}
	return false
}

func run(pass *analysis.Pass) (interface{}, error) {
	expr := func(x *ast.ExprStmt) {
		// проверяем, что выражение представляет собой вызов функции,
		// у которой возвращаемая ошибка никак не обрабатывается
		if call, ok := x.X.(*ast.CallExpr); ok {
			if isReturnError(pass, call) {
				pass.Reportf(x.Pos(), "expression returns unchecked error")
			}
		}
	}	

	FuncMainsearchEnd := func(x *ast.FuncDecl) /*(endPos token.Pos)*/{		
			//pass.Reportf(x.Pos(), x.Name.String())
			//pass.Reportf(x.End(), "end of func")
			//fmt.Println(pass.Fset.Position(x.End()).Line)
			FuncMainEnd = (pass.Fset.Position(x.End()).Line)
	}

	funcIdent := func(x *ast.Ident) {		
		if pass.Fset.Position(x.Pos()).Line <= FuncMainEnd {
			pass.Reportf(x.Pos(),"os." + x.Name + " is forbidden to call from main function of main pkg ")
		}
	}

/*
	funcCalled := func(x *ast.CallExpr) {		
		if call, ok := x.Fun.(*ast.SelectorExpr); ok {
			pass.Reportf(x.Pos(), call.Sel.Name)
		}
		
	}
*/
	tuplefunc := func(x *ast.AssignStmt) {

		// рассматриваем присваивание, при котором
		// вместо получения ошибок используется '_'
		// a, b, _ := tuplefunc()
		// проверяем, что это вызов функции
		if call, ok := x.Rhs[0].(*ast.CallExpr); ok {
			results := resultErrors(pass, call)
			for i := 0; i < len(x.Lhs); i++ {
				// перебираем все идентификаторы слева от присваивания
				if id, ok := x.Lhs[i].(*ast.Ident); ok && id.Name == "_" && results[i] {
					pass.Reportf(id.NamePos, "assignment with unchecked error")
				}
			}
		}
	}
	errfunc := func(x *ast.AssignStmt) {
		// множественное присваивание: a, _ := b, myfunc()
		// ищем ситуацию, когда функция справа возвращает ошибку,
		// а соответствующий идентификатор слева равен '_'
		for i := 0; i < len(x.Lhs); i++ {
			if id, ok := x.Lhs[i].(*ast.Ident); ok {
				// вызов функции справа
				if call, ok := x.Rhs[i].(*ast.CallExpr); ok {
					if id.Name == "_" && isReturnError(pass, call) {
						pass.Reportf(id.NamePos, "assignment with unchecked error")
					}
				}
			}
		}
	}
	//n.(*ast.FuncType)
	lastIdent := ""
	for _, file := range pass.Files {
	if file.Name.Name == "main" {	
		// функцией ast.Inspect проходим по всем узлам AST
		ast.Inspect(file, func(node ast.Node) bool {
			//pass.Reportf(file.Pos(),file.Name.Name)
			switch x := node.(type) {
			case *ast.ExprStmt: // выражение
				expr(x)

			case *ast.FuncDecl:
				if x.Name.String() == "main"{
					FuncMainsearchEnd(x)			
				}
			//case *ast.CallExpr:	
			//case *ast.SelectorExpr:
			//	funcCalled(x)

			case *ast.Ident:
				if x.Name == "os" {
					lastIdent = "os"
				} else {
					if lastIdent == "os" && x.Name == "Exit"{	
						lastIdent = ""			
						funcIdent(x)
					} else {
						lastIdent = ""
					}
				}

				

			case *ast.GoStmt: // go myfunc()
				if isReturnError(pass, x.Call) {
					pass.Reportf(x.Pos(), "go statement with unchecked error")
				}

            case *ast.DeferStmt: // defer myfunc()
                if isReturnError(pass, x.Call) {
                    pass.Reportf(x.Pos(), "defer with unchecked error")
                }
				
			case *ast.AssignStmt: // оператор присваивания
				// справа одно выражение x,y := myfunc()
				if len(x.Rhs) == 1 {
					tuplefunc(x)
				} else {
					// справа несколько выражений x,y := z,myfunc()
					errfunc(x)
				}
			}
			return true
		})
	}
	}
	return nil, nil
}