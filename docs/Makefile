lingo.pdf: lingo.tex
	rubber -d lingo

lingo.tex: intro.md basics.md permissions.md abstract-interpreter.md evaluation.md conclusion.md meta.yaml
	pandoc -N -o $@ --biblatex  --bibliography=thesis.bib  --listings --include-after-body=declaration.tex $^
