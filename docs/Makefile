lingo.pdf: lingo.tex
	rubber -d lingo

lingo.tex: declaration.tex intro.md basics.md permissions.md abstract-interpreter.md evaluation.md conclusion.md meta.yaml
	pandoc -N -o $@ --biblatex  --bibliography=thesis.bib  --listings --include-after-body=$^

clean:
	rm -f *.aux *.bbl *.bcf *.blg *.dvi *.log *.out *.toc *.run.xml
