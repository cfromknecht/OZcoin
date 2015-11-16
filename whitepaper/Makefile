.SECONDARY:
.PHONY: all clean

PDFLATEX := $(shell which pdflatex)
BIBTEX   := $(shell which bibtex)
TARGET   := zerosum.pdf

all: $(TARGET)

%.pdf: %.tex
	@echo $(PDFLATEX) $*
	@$(PDFLATEX) $*
	@$(BIBTEX) $*
	@$(PDFLATEX) $*
	@$(PDFLATEX) $*
	@open $@

clean:
	rm -f *.aux *.ps *.pdf *.dvi *.bbl *.blg *.tmp *.log *~
