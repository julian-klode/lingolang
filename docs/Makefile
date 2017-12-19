INPUT=intro.md basics.md permissions.md abstract-interpreter.md evaluation.md conclusion.md meta.yaml

lingo.pdf: lingo.tex
	rubber -d lingo

lingo.tex: declaration.tex thesis.bib $(INPUT)
	pandoc -N -o $@ --biblatex  --bibliography=thesis.bib  --listings --include-after-body=declaration.tex $(INPUT)

clean:
	rm -f *.aux *.bbl *.bcf *.blg *.dvi *.log *.out *.toc *.run.xml
