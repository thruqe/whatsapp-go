package main

// SupportedLanguages defines the list of locales with full or partial bot translation strings.
var SupportedLanguages = map[string]string{
	"en": "English",
	"es": "Spanish (Español)",
	"id": "Indonesian (Bahasa Indonesia)",
	"pt": "Portuguese (Português)",
	"fr": "French (Français)",
	"ar": "Arabic (العربية)",
	"ru": "Russian (Русский)",
	"de": "German (Deutsch)",
}

// LocaleCatalog maps locale code -> key -> localized text format string.
// No emojis, no WhatsApp markdown formatting symbols (*, _, ~, ```).
var LocaleCatalog = map[string]map[string]string{
	"en": {
		// Common responses & errors
		"common.error":             "An error occurred while processing your request.",
		"common.unauthorized":      "You are not authorized to use this command.",
		"common.admin_only":        "Only group admins can use this command.",
		"common.owner_only":        "Only the bot owner can use this command.",
		"common.group_only":        "This command can only be used in group chats.",
		"common.dm_only":           "This command can only be used in private messages.",
		"common.invalid_args":      "Invalid arguments provided.",
		"common.store_unavailable": "Database store is currently unavailable.",
		"common.enabled":           "enabled",
		"common.disabled":          "disabled",
		"common.none":              "none",
		"common.unknown":           "unknown",

		// Ping command
		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		// Uptime & Info
		"uptime.active": "Bot has been active for: %s",
		"repo.desc":     "WhatsRook WhatsApp automation engine and bot.",

		// Language command
		"lang.current":        "Current language: %s (%s)",
		"lang.set_success":    "Language successfully updated to %s (%s).",
		"lang.invalid":        "Invalid language code: %s.",
		"lang.available_list": "Available languages:\n%s\n\nUsage: %slang <code> (e.g. %slang es)",
		"lang.reset":          "Language preference reset to default.",
		"lang.detect_info":    "Auto-detected language from phone country: %s (%s).",
	},

	"es": {
		"common.error":             "Ocurrio un error al procesar tu solicitud.",
		"common.unauthorized":      "No estas autorizado para usar este comando.",
		"common.admin_only":        "Solo los administradores del grupo pueden usar este comando.",
		"common.owner_only":        "Solo el propietario del bot puede usar este comando.",
		"common.group_only":        "Este comando solo se puede usar en grupos.",
		"common.dm_only":           "Este comando solo se puede usar en mensajes privados.",
		"common.invalid_args":      "Argumentos invalidos proporcionados.",
		"common.store_unavailable": "La base de datos no esta disponible actualmente.",
		"common.enabled":           "activado",
		"common.disabled":          "desactivado",
		"common.none":              "ninguno",
		"common.unknown":           "desconocido",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "El bot ha estado activo durante: %s",
		"repo.desc":     "WhatsRook motor de automatizacion de WhatsApp y bot.",

		"lang.current":        "Idioma actual: %s (%s)",
		"lang.set_success":    "Idioma actualizado correctamente a %s (%s).",
		"lang.invalid":        "Codigo de idioma no valido: %s.",
		"lang.available_list": "Idiomas disponibles:\n%s\n\nUso: %slang <codigo> (ej. %slang es)",
		"lang.reset":          "Preferencia de idioma restablecida al valor predeterminado.",
		"lang.detect_info":    "Idioma detectado automaticamente por prefijo telefonico: %s (%s).",
	},

	"id": {
		"common.error":             "Terjadi kesalahan saat memproses permintaan Anda.",
		"common.unauthorized":      "Anda tidak memiliki izin untuk menggunakan perintah ini.",
		"common.admin_only":        "Hanya admin grup yang dapat menggunakan perintah ini.",
		"common.owner_only":        "Hanya pemilik bot yang dapat menggunakan perintah ini.",
		"common.group_only":        "Perintah ini hanya dapat digunakan di grup.",
		"common.dm_only":           "Perintah ini hanya dapat digunakan di pesan pribadi.",
		"common.invalid_args":      "Argumen yang diberikan tidak valid.",
		"common.store_unavailable": "Database penyimpanan saat ini tidak tersedia.",
		"common.enabled":           "aktif",
		"common.disabled":          "nonaktif",
		"common.none":              "tidak ada",
		"common.unknown":           "tidak diketahui",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "Bot telah aktif selama: %s",
		"repo.desc":     "WhatsRook mesin otomatisasi dan bot WhatsApp.",

		"lang.current":        "Bahasa saat ini: %s (%s)",
		"lang.set_success":    "Bahasa berhasil diubah ke %s (%s).",
		"lang.invalid":        "Kode bahasa tidak valid: %s.",
		"lang.available_list": "Bahasa yang tersedia:\n%s\n\nPenggunaan: %slang <kode> (contoh: %slang id)",
		"lang.reset":          "Preferensi bahasa diatur ulang ke default.",
		"lang.detect_info":    "Bahasa terdeteksi otomatis dari kode negara telepon: %s (%s).",
	},

	"pt": {
		"common.error":             "Ocorreu um erro ao processar sua solicitacao.",
		"common.unauthorized":      "Voce nao tem autorizacao para usar este comando.",
		"common.admin_only":        "Apenas administradores do grupo podem usar este comando.",
		"common.owner_only":        "Apenas o proprietario do bot pode usar este comando.",
		"common.group_only":        "Este comando so pode ser usado em grupos.",
		"common.dm_only":           "Este comando so pode ser usado em mensagens privadas.",
		"common.invalid_args":      "Argumentos invalidos fornecidos.",
		"common.store_unavailable": "O banco de dados nao esta disponivel no momento.",
		"common.enabled":           "ativado",
		"common.disabled":          "desativado",
		"common.none":              "nenhum",
		"common.unknown":           "desconhecido",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "O bot esta ativo ha: %s",
		"repo.desc":     "WhatsRook motor de automacao e bot do WhatsApp.",

		"lang.current":        "Idioma atual: %s (%s)",
		"lang.set_success":    "Idioma alterado com sucesso para %s (%s).",
		"lang.invalid":        "Codigo de idioma invalido: %s.",
		"lang.available_list": "Idiomas disponiveis:\n%s\n\nUso: %slang <codigo> (ex: %slang pt)",
		"lang.reset":          "Preferencia de idioma redefinida para o padrao.",
		"lang.detect_info":    "Idioma detectado automaticamente pelo codigo do pais: %s (%s).",
	},

	"fr": {
		"common.error":             "Une erreur est survenue lors du traitement de votre demande.",
		"common.unauthorized":      "Vous n etes pas autorise a utiliser cette commande.",
		"common.admin_only":        "Seuls les administrateurs du groupe peuvent utiliser cette commande.",
		"common.owner_only":        "Seul le proprietaire du bot peut utiliser cette commande.",
		"common.group_only":        "Cette commande ne peut etre utilisee que dans les groupes.",
		"common.dm_only":           "Cette commande ne peut etre utilisee qu en message prive.",
		"common.invalid_args":      "Arguments invalides fournis.",
		"common.store_unavailable": "La base de donnees est actuellement indisponible.",
		"common.enabled":           "active",
		"common.disabled":          "desactive",
		"common.none":              "aucun",
		"common.unknown":           "inconnu",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "Le bot est actif depuis: %s",
		"repo.desc":     "WhatsRook moteur d automatisation et bot WhatsApp.",

		"lang.current":        "Langue actuelle: %s (%s)",
		"lang.set_success":    "Langue mise a jour avec succes: %s (%s).",
		"lang.invalid":        "Code de langue invalide: %s.",
		"lang.available_list": "Langues disponibles:\n%s\n\nUtilisation: %slang <code_langue> (ex: %slang fr)",
		"lang.reset":          "Preference de langue reinitialisee par defaut.",
		"lang.detect_info":    "Langue detectee automatiquement par indicatif telephonique: %s (%s).",
	},

	"ar": {
		"common.error":             "حدث خطأ أثناء معالجة طلبك.",
		"common.unauthorized":      "ليس لديك إذن لاستخدام هذا الأمر.",
		"common.admin_only":        "مشرفو المجموعة فقط هم من يمكنهم استخدام هذا الأمر.",
		"common.owner_only":        "مالك البوت فقط يمكنه استخدام هذا الأمر.",
		"common.group_only":        "لا يمكن استخدام هذا الأمر إلا في المجموعات.",
		"common.dm_only":           "لا يمكن استخدام هذا الأمر إلا في الرسائل الخاصة.",
		"common.invalid_args":      "المعلمات المقدمة غير صالحة.",
		"common.store_unavailable": "قاعدة البيانات غير متاحة حاليا.",
		"common.enabled":           "مفعل",
		"common.disabled":          "معطل",
		"common.none":              "لا يوجد",
		"common.unknown":           "غير معروف",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "البوت نشط منذ: %s",
		"repo.desc":     "WhatsRook محرك أتمتة وتطبيق واتساب.",

		"lang.current":        "اللغة الحالية: %s (%s)",
		"lang.set_success":    "تم تحديث اللغة بنجاح إلى %s (%s).",
		"lang.invalid":        "رمز لغة غير صالح: %s.",
		"lang.available_list": "اللغات المتاحة:\n%s\n\nالاستخدام: %slang <الرمز> (مثال: %slang ar)",
		"lang.reset":          "تمت إعادة تعيين تفضيل اللغة إلى الافتراضي.",
		"lang.detect_info":    "تم اكتشاف اللغة تلقائيا من رمز الدولة: %s (%s).",
	},

	"ru": {
		"common.error":             "Произошла ошибка при обработке вашего запроса.",
		"common.unauthorized":      "У вас нет прав для выполнения этой команды.",
		"common.admin_only":        "Эту команду могут использовать только администраторы группы.",
		"common.owner_only":        "Эту команду может использовать только владелец бота.",
		"common.group_only":        "Эту команду можно использовать только в группах.",
		"common.dm_only":           "Эту команду можно использовать только в личных сообщениях.",
		"common.invalid_args":      "Указаны неверные параметры.",
		"common.store_unavailable": "База данных в данный момент недоступна.",
		"common.enabled":           "включено",
		"common.disabled":          "выключено",
		"common.none":              "нет",
		"common.unknown":           "неизвестно",

		"ping.pong_ms": "Понг! %d мс",
		"ping.pong_us": "Понг! %d мкс",
		"ping.pong_ns": "Понг! %d нс",

		"uptime.active": "Бот активен уже: %s",
		"repo.desc":     "WhatsRook автоматизация и бот для WhatsApp.",

		"lang.current":        "Текущий язык: %s (%s)",
		"lang.set_success":    "Язык успешно изменен на %s (%s).",
		"lang.invalid":        "Неверный код языка: %s.",
		"lang.available_list": "Доступные языки:\n%s\n\nИспользование: %slang <код> (например: %slang ru)",
		"lang.reset":          "Настройки языка сброшены по умолчанию.",
		"lang.detect_info":    "Язык автоматически определен по номеру телефона: %s (%s).",
	},

	"de": {
		"common.error":             "Bei der Bearbeitung Ihrer Anfrage ist ein Fehler aufgetreten.",
		"common.unauthorized":      "Sie sind nicht berechtigt, diesen Befehl zu verwenden.",
		"common.admin_only":        "Nur Gruppenadministratoren koennen diesen Befehl verwenden.",
		"common.owner_only":        "Nur der Bot-Besitzer kann diesen Befehl verwenden.",
		"common.group_only":        "Dieser Befehl kann nur in Gruppen verwendet werden.",
		"common.dm_only":           "Dieser Befehl kann nur in privaten Nachrichten verwendet werden.",
		"common.invalid_args":      "Ungueltige Argumente angegeben.",
		"common.store_unavailable": "Die Datenbank ist derzeit nicht verfuegbar.",
		"common.enabled":           "aktiviert",
		"common.disabled":          "deaktiviert",
		"common.none":              "keine",
		"common.unknown":           "unbekannt",

		"ping.pong_ms": "Pong! %d ms",
		"ping.pong_us": "Pong! %d us",
		"ping.pong_ns": "Pong! %d ns",

		"uptime.active": "Bot ist aktiv seit: %s",
		"repo.desc":     "WhatsRook WhatsApp-Automatisierung und Bot.",

		"lang.current":        "Aktuelle Sprache: %s (%s)",
		"lang.set_success":    "Sprache erfolgreich auf %s (%s) aktualisiert.",
		"lang.invalid":        "Ungueltiger Sprachcode: %s.",
		"lang.available_list": "Verfuegbare Sprachen:\n%s\n\nVerwendung: %slang <code> (z.B. %slang de)",
		"lang.reset":          "Spracheinstellung auf Standard zurueckgesetzt.",
		"lang.detect_info":    "Sprache anhand der Telefonlaendervorwahl automatisch erkannt: %s (%s).",
	},
}
