# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# The Unix install contract: make, make test, make install, make uninstall,
# honouring PREFIX and DESTDIR and installing to FHS paths.
#
# GNU make reads GNUmakefile in preference to Makefile, so this file includes
# the development Makefile rather than shadowing it: `make test`, `make lint`
# and every other dev target keep working exactly as before, and the packaging
# targets are additive.
include Makefile

PREFIX      ?= /usr/local
BINDIR      ?= $(PREFIX)/bin
DATADIR     ?= $(PREFIX)/share
MANDIR      ?= $(DATADIR)/man
DOCDIR      ?= $(DATADIR)/doc/$(BINARY)
LICENSEDIR  ?= $(DATADIR)/licenses/$(BINARY)

BASHCOMPDIR ?= $(DATADIR)/bash-completion/completions
ZSHCOMPDIR  ?= $(DATADIR)/zsh/site-functions
FISHCOMPDIR ?= $(DATADIR)/fish/vendor_completions.d

INSTALL         ?= install
INSTALL_PROGRAM ?= $(INSTALL) -m 0755
INSTALL_DATA    ?= $(INSTALL) -m 0644

# Generated, never committed: a manpage or completion checked into the tree is
# a copy of the CLI that nothing keeps honest.
GEN := $(BIN_DIR)/gen

.PHONY: generated
generated: build ## Generate the manpage and shell completions into bin/gen
	@mkdir -p $(GEN)
	$(BIN_DIR)/$(BINARY) --man                > $(GEN)/$(BINARY).1
	$(BIN_DIR)/$(BINARY) --completion bash    > $(GEN)/$(BINARY).bash
	$(BIN_DIR)/$(BINARY) --completion zsh     > $(GEN)/_$(BINARY)
	$(BIN_DIR)/$(BINARY) --completion fish    > $(GEN)/$(BINARY).fish

.PHONY: install
install: generated ## Install to $(DESTDIR)$(PREFIX) using FHS paths
	$(INSTALL) -d $(DESTDIR)$(BINDIR)
	$(INSTALL_PROGRAM) $(BIN_DIR)/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)

	$(INSTALL) -d $(DESTDIR)$(MANDIR)/man1
	$(INSTALL_DATA) $(GEN)/$(BINARY).1 $(DESTDIR)$(MANDIR)/man1/$(BINARY).1

	$(INSTALL) -d $(DESTDIR)$(BASHCOMPDIR)
	$(INSTALL_DATA) $(GEN)/$(BINARY).bash $(DESTDIR)$(BASHCOMPDIR)/$(BINARY)
	$(INSTALL) -d $(DESTDIR)$(ZSHCOMPDIR)
	$(INSTALL_DATA) $(GEN)/_$(BINARY) $(DESTDIR)$(ZSHCOMPDIR)/_$(BINARY)
	$(INSTALL) -d $(DESTDIR)$(FISHCOMPDIR)
	$(INSTALL_DATA) $(GEN)/$(BINARY).fish $(DESTDIR)$(FISHCOMPDIR)/$(BINARY).fish

	$(INSTALL) -d $(DESTDIR)$(DOCDIR)
	$(INSTALL_DATA) README.md CHANGELOG.md $(DESTDIR)$(DOCDIR)/

	$(INSTALL) -d $(DESTDIR)$(LICENSEDIR)
	$(INSTALL_DATA) LICENSE-MIT LICENSE-APACHE $(DESTDIR)$(LICENSEDIR)/

.PHONY: uninstall
uninstall: ## Remove everything install put down
	rm -f  $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f  $(DESTDIR)$(MANDIR)/man1/$(BINARY).1
	rm -f  $(DESTDIR)$(BASHCOMPDIR)/$(BINARY)
	rm -f  $(DESTDIR)$(ZSHCOMPDIR)/_$(BINARY)
	rm -f  $(DESTDIR)$(FISHCOMPDIR)/$(BINARY).fish
	rm -rf $(DESTDIR)$(DOCDIR)
	rm -rf $(DESTDIR)$(LICENSEDIR)
