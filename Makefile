# Configurações globais
APP_ID      := com.github.gabrielmoura.nostrFilePublisher
APP_NAME    := nostrFilePublisher
APP_VERSION := 0.1.0
ICON        := icons/Icon.png

# Plataformas suportadas
PLATFORMS   := android ios windows linux

# Alvo padrão → mostra help
.DEFAULT_GOAL := help

# Exibe comandos disponíveis
help:
	@echo "📦 Nostr File Publisher - Build System"
	@echo
	@echo "Comandos disponíveis:"
	@echo "  make android   → Build para Android"
	@echo "  make ios       → Build para iOS"
	@echo "  make windows   → Build para Windows"
	@echo "  make linux     → Build para Linux"
	@echo "  make all       → Build para todas as plataformas"
	@echo "  make clean     → Limpa os artefatos de build"
	@echo

# Build para todas as plataformas
all: $(PLATFORMS)
	@echo "✅ Build completo para todas as plataformas!"

# Regra genérica para cada plataforma
$(PLATFORMS):
	@echo "🔨 Iniciando build para $@..."
	fyne package --os $@ \
		--app-id $(APP_ID) \
		--icon $(ICON) \
		--app-version $(APP_VERSION) \
		--name $(APP_NAME)
	@echo "✅ Build para $@ concluído."

# Limpeza
clean:
	@echo "🧹 Limpando builds..."
	rm -rf $(APP_NAME).exe $(APP_NAME).apk $(APP_NAME).app $(APP_NAME).tar.xz
	@echo "✅ Limpeza concluída."

.PHONY: help all clean $(PLATFORMS)
