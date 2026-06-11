package i18n

import "fmt"

// Lang is the language code stored per chat.
type Lang = string

const (
	EN Lang = "en"
	UK Lang = "uk"
)

// T returns the translated string for key in lang. Falls back to English.
func T(lang Lang, key string) string {
	if m, ok := translations[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if s, ok := translations[EN][key]; ok {
		return s
	}
	return key
}

// Tf formats a translated string with printf-style args.
func Tf(lang Lang, key string, args ...interface{}) string {
	return fmt.Sprintf(T(lang, key), args...)
}

var translations = map[Lang]map[string]string{
	EN: {
		// Action labels [FEAT-004]
		"action_delete": "message removed, no further action",
		"action_mute":   "muted for %d hour(s)",
		"action_kick":   "kicked from group (can rejoin via invite)",
		"action_ban":    "permanently banned",

		// Verification [FEAT-015]
		"verify_message": "<b>Welcome, %s!</b>\n\nPlease tap the button below to confirm you are a real person.\n" +
			"You have <b>%d minute(s)</b> to verify — if you don't respond, you will be automatically removed.",
		"verify_button":        "I'm not a bot — let me in",
		"verify_popup_invalid": "This button is not for you.",
		"verify_popup_ok":      "Verified! Welcome to the group.",

		// Spam notification [FEAT-014]
		"spam_notification": "<b>Spam removed</b>\nUser: %s\nMatched %s: <code>%s</code>\nAction: %s",

		// Confirm callback
		"confirm_admin_only": "Only the admin who ran the command can confirm.",
		"confirm_cancelled":  "Cancelled.",

		// Clear words
		"clear_words_fail":    "Failed to clear the word list.",
		"clear_words_ok":      "Word list cleared.",
		"clear_words_ok_full": "Word list cleared. No words are blocked in this chat.",
		// Clear regex
		"clear_regex_fail":    "Failed to clear regex patterns.",
		"clear_regex_ok":      "Regex list cleared.",
		"clear_regex_ok_full": "Regex list cleared. No patterns are active in this chat.",
		// Clear log
		"clear_log_fail": "Failed to clear the spam log.",
		"clear_log_ok":   "Spam log cleared.",
		// Copy result
		"copy_result_fail": "Copy failed: %v",
		"copy_result_ok":   "Settings copied from <code>%d</code> to <code>%d</code>. The target chat now has the same words, patterns, and settings.",

		// Add [FEAT-001, FEAT-002, FEAT-011]
		"add_word_usage":      "Usage:\n<code>/tgwatch_add word bitcoin</code>\nor multiline:\n<code>/tgwatch_add word\nbitcoin\nusdt\nкрипта</code>",
		"add_word_fail":       "Failed to add words.",
		"add_word_ok":         "Added <b>%d</b> word(s).",
		"add_word_ok_skipped": "Added <b>%d</b> word(s). <b>%d</b> already existed and were skipped.",
		"add_regex_usage":     "Usage: <code>/tgwatch_add regex &lt;pattern&gt;</code>",
		"add_regex_invalid":   "Invalid regex pattern:\n<code>%s</code>",
		"add_regex_fail":      "Failed to add regex pattern.",
		"add_regex_ok":        "Regex pattern added:\n<code>%s</code>",
		"add_regex_exists":    "That pattern is already in the list.",
		"add_usage":           "Usage: /tgwatch_add word &lt;text&gt;  or  /tgwatch_add regex &lt;pattern&gt;",

		// Remove
		"remove_usage":          "Usage: /tgwatch_remove word &lt;text&gt; or /tgwatch_remove regex &lt;pattern&gt;",
		"remove_word_fail":      "Failed to remove word.",
		"remove_word_ok":        "Removed word: <code>%s</code>",
		"remove_word_notfound":  "Word not found: <code>%s</code>",
		"remove_regex_fail":     "Failed to remove regex pattern.",
		"remove_regex_ok":       "Removed pattern: <code>%s</code>",
		"remove_regex_notfound": "Pattern not found: <code>%s</code>",
		"remove_usage_sub":      "Usage: /tgwatch_remove word &lt;text&gt;  or  /tgwatch_remove regex &lt;pattern&gt;",

		// List
		"list_words_fail":   "Failed to fetch the word list.",
		"list_words_empty":  "No blocked words yet. Use /tgwatch_add word to add some.",
		"list_words_header": "<b>Blocked words</b> (%d):\n<code>%s</code>",
		"list_regex_fail":   "Failed to fetch regex patterns.",
		"list_regex_empty":  "No regex patterns yet. Use /tgwatch_add regex to add some.",
		"list_regex_header": "<b>Regex patterns</b> (%d):\n%s",
		"list_usage":        "Usage: /tgwatch_list words  or  /tgwatch_list regex",

		// Clear command buttons
		"btn_cancel":         "Cancel",
		"btn_clear_words":    "Yes, clear all words",
		"btn_clear_regex":    "Yes, clear all patterns",
		"btn_clear_log":      "Yes, clear log",
		"clear_words_prompt": "This will remove <b>all blocked words</b> for this chat.",
		"clear_regex_prompt": "This will remove <b>all regex patterns</b> for this chat.",
		"clear_usage":        "Usage: /tgwatch_clear words  or  /tgwatch_clear regex",

		// Set
		"set_usage": "Usage: /tgwatch_set &lt;action|mute_duration|sandbox|sandbox_duration|name_filter|verification|verification_timeout&gt; &lt;value&gt;",
		"set_action_valid": "Valid actions:\n<code>delete</code> — remove message only\n<code>mute</code> — remove + mute for N hours\n<code>kick</code> — remove + kick (can rejoin)\n<code>ban</code> — remove + permanent ban",
		"set_action_fail":          "Failed to update action.",
		"set_action_ok":            "Spam action set to <b>%s</b>.",
		"set_mute_invalid":         "Mute duration must be a positive whole number of hours.",
		"set_mute_fail":            "Failed to update mute duration.",
		"set_mute_ok":              "Mute duration set to <b>%d hour(s)</b>.",
		"set_sandbox_enable_fail":  "Failed to enable sandbox.",
		"set_sandbox_enabled":      "Sandbox enabled. New members will be restricted until the sandbox period expires.",
		"set_sandbox_disable_fail": "Failed to disable sandbox.",
		"set_sandbox_disabled":     "Sandbox disabled. New members can post immediately.",
		"set_sandbox_usage":        "Usage: /tgwatch_set sandbox on|off",
		"set_sandbox_dur_invalid":  "Sandbox duration must be a positive whole number of hours.",
		"set_sandbox_dur_fail":     "Failed to update sandbox duration.",
		"set_sandbox_dur_ok":       "Sandbox duration set to <b>%d hour(s)</b>.",
		"set_nf_enable_fail":       "Failed to enable name filter.",
		"set_nf_enabled":           "Name filter enabled. New members with spammy names will be kicked on join.",
		"set_nf_disable_fail":      "Failed to disable name filter.",
		"set_nf_disabled":          "Name filter disabled.",
		"set_nf_usage":             "Usage: /tgwatch_set name_filter on|off",
		"set_verif_enable_fail":    "Failed to enable verification.",
		"set_verif_enabled":        "Verification enabled. New members must tap a button before they can post.",
		"set_verif_disable_fail":   "Failed to disable verification.",
		"set_verif_disabled":       "Verification disabled. New members can post immediately (subject to sandbox settings).",
		"set_verif_usage":          "Usage: /tgwatch_set verification on|off",
		"set_verif_to_invalid":     "Verification timeout must be a positive whole number of minutes.",
		"set_verif_to_fail":        "Failed to update verification timeout.",
		"set_verif_to_ok":          "Verification timeout set to <b>%d minute(s)</b>.",
		"set_unknown":              "Unknown setting. Valid keys:\n<code>action</code>, <code>mute_duration</code>, <code>sandbox</code>, <code>sandbox_duration</code>, <code>name_filter</code>, <code>verification</code>, <code>verification_timeout</code>",

		// Show settings
		"show_settings_fail": "Error fetching settings.",
		"show_settings": "<b>Settings for this chat</b>\n\n" +
			"Spam action:           <b>%s</b>\n" +
			"Mute duration:         <b>%dh</b>\n" +
			"Name filter:           <b>%s</b>\n" +
			"Verification:          <b>%s</b>\n" +
			"Verification timeout:  <b>%dm</b>\n" +
			"Sandbox:               <b>%s</b>\n" +
			"Sandbox duration:      <b>%dh</b>\n" +
			"Language:              <b>%s</b>",
		"settings_on":  "on",
		"settings_off": "off",
		"settings_lang_en": "🇬🇧 English",
		"settings_lang_uk": "🇺🇦 Ukrainian",

		// Check [FEAT-008]
		"check_usage":      "Usage: /tgwatch_check &lt;message text&gt;",
		"check_fail":       "Error running check.",
		"check_cfg_fail":   "Error fetching settings.",
		"check_no_match":   "No match — this message would pass through.",
		"check_match": "<b>Match found</b>\n" +
			"Type:    <b>%s</b>\n" +
			"Pattern: <code>%s</code>\n" +
			"Normalized input: <code>%s</code>\n" +
			"Would action: <b>%s</b>",

		// Unrestrict [FEAT-007]
		"unrestrict_usage": "Reply to the user's message to unrestrict them.",
		"unrestrict_ok":    "User <b>#%d</b> has been unrestricted and can post again.",

		// Log [FEAT-009]
		"log_clear_prompt": "This will clear the <b>entire spam log</b> for this chat.",
		"log_usage":        "Usage: /tgwatch_log [n|clear]",
		"log_fail":         "Error fetching spam log.",
		"log_empty":        "Spam log is empty.",
		"log_header":       "<b>Last %d spam entries:</b>\n\n%s",

		// Copy [FEAT-012]
		"copy_usage":       "Usage: /tgwatch_copy &lt;source_chat_id&gt; &lt;target_chat_id&gt;",
		"copy_invalid_ids": "Chat IDs must be numeric.",
		"copy_prompt": "Copy all settings from <code>%d</code> to <code>%d</code>?\n" +
			"<i>This will overwrite all words, patterns, and settings in the target chat.</i>\n" +
			"Reply <b>YES</b> to confirm (60s timeout).",

		// Lang command [FEAT-016]
		"lang_prompt": "Choose the bot language for this chat:",
		"btn_lang_en": "🇬🇧 English",
		"btn_lang_uk": "🇺🇦 Українська",
		"lang_set_en": "Language set to 🇬🇧 English.",
		"lang_set_uk": "Language set to 🇺🇦 Українська.",
		"lang_fail":   "Failed to save language setting.",

		// Help
		"help": `<b>tgwatchspam commands</b>

<b>Filters</b>
/tgwatch_add word &lt;text&gt; — add blocked word/phrase
/tgwatch_add regex &lt;pattern&gt; — add RE2 regex pattern
/tgwatch_remove word &lt;text&gt; — remove word
/tgwatch_remove regex &lt;pattern&gt; — remove pattern
/tgwatch_list words — list blocked words
/tgwatch_list regex — list regex patterns
/tgwatch_clear words — remove all words
/tgwatch_clear regex — remove all patterns

<b>Actions</b>
/tgwatch_set action delete — remove message only
/tgwatch_set action mute — remove + mute for N hours
/tgwatch_set action kick — remove + kick (can rejoin)
/tgwatch_set action ban — remove + permanent ban
/tgwatch_set mute_duration &lt;hours&gt; — mute duration (default 24h)

<b>New member controls</b>
/tgwatch_set sandbox on|off
/tgwatch_set sandbox_duration &lt;hours&gt;
/tgwatch_set verification on|off
/tgwatch_set verification_timeout &lt;minutes&gt;
/tgwatch_set name_filter on|off
/tgwatch_unrestrict — reply to lift sandbox on a user

<b>Management</b>
/tgwatch_show settings — show current config
/tgwatch_check &lt;text&gt; — test text against filters
/tgwatch_log [n] — show last N spam entries
/tgwatch_log clear — clear spam log
/tgwatch_clean — delete all bot replies and admin commands
/tgwatch_lang — change bot language (🇬🇧 / 🇺🇦)`,
	},

	UK: {
		// Action labels [FEAT-004]
		"action_delete": "повідомлення видалено, без подальших дій",
		"action_mute":   "замовчано на %d год.",
		"action_kick":   "виключено з групи (може повернутись за запрошенням)",
		"action_ban":    "заблоковано назавжди",

		// Verification [FEAT-015]
		"verify_message": "<b>Ласкаво просимо, %s!</b>\n\nНатисніть кнопку нижче, щоб підтвердити, що ви — реальна людина.\n" +
			"У вас є <b>%d хв.</b> на верифікацію — якщо не відповісте, вас буде видалено автоматично.",
		"verify_button":        "Я не бот — впустіть мене",
		"verify_popup_invalid": "Ця кнопка не для вас.",
		"verify_popup_ok":      "Верифіковано! Ласкаво просимо до групи.",

		// Spam notification [FEAT-014]
		"spam_notification": "<b>Спам видалено</b>\nКористувач: %s\nЗбіг %s: <code>%s</code>\nДія: %s",

		// Confirm callback
		"confirm_admin_only": "Лише адміністратор, який виконав команду, може підтвердити.",
		"confirm_cancelled":  "Скасовано.",

		// Clear words
		"clear_words_fail":    "Не вдалося очистити список слів.",
		"clear_words_ok":      "Список слів очищено.",
		"clear_words_ok_full": "Список слів очищено. Жодне слово не заблоковано в цьому чаті.",
		// Clear regex
		"clear_regex_fail":    "Не вдалося очистити regex-шаблони.",
		"clear_regex_ok":      "Список regex очищено.",
		"clear_regex_ok_full": "Список regex очищено. Жоден шаблон не активний у цьому чаті.",
		// Clear log
		"clear_log_fail": "Не вдалося очистити журнал спаму.",
		"clear_log_ok":   "Журнал спаму очищено.",
		// Copy result
		"copy_result_fail": "Помилка копіювання: %v",
		"copy_result_ok":   "Налаштування скопійовано з <code>%d</code> до <code>%d</code>. Цільовий чат тепер має ті самі слова, шаблони та налаштування.",

		// Add [FEAT-001, FEAT-002, FEAT-011]
		"add_word_usage":      "Використання:\n<code>/tgwatch_add word bitcoin</code>\nабо багаторядково:\n<code>/tgwatch_add word\nbitcoin\nusdt\nкрипта</code>",
		"add_word_fail":       "Не вдалося додати слова.",
		"add_word_ok":         "Додано <b>%d</b> слів(о).",
		"add_word_ok_skipped": "Додано <b>%d</b> слів(о). <b>%d</b> вже існували та були пропущені.",
		"add_regex_usage":     "Використання: <code>/tgwatch_add regex &lt;шаблон&gt;</code>",
		"add_regex_invalid":   "Некоректний regex-шаблон:\n<code>%s</code>",
		"add_regex_fail":      "Не вдалося додати regex-шаблон.",
		"add_regex_ok":        "Regex-шаблон додано:\n<code>%s</code>",
		"add_regex_exists":    "Цей шаблон вже є в списку.",
		"add_usage":           "Використання: /tgwatch_add word &lt;текст&gt;  або  /tgwatch_add regex &lt;шаблон&gt;",

		// Remove
		"remove_usage":          "Використання: /tgwatch_remove word &lt;текст&gt; або /tgwatch_remove regex &lt;шаблон&gt;",
		"remove_word_fail":      "Не вдалося видалити слово.",
		"remove_word_ok":        "Слово видалено: <code>%s</code>",
		"remove_word_notfound":  "Слово не знайдено: <code>%s</code>",
		"remove_regex_fail":     "Не вдалося видалити regex-шаблон.",
		"remove_regex_ok":       "Шаблон видалено: <code>%s</code>",
		"remove_regex_notfound": "Шаблон не знайдено: <code>%s</code>",
		"remove_usage_sub":      "Використання: /tgwatch_remove word &lt;текст&gt;  або  /tgwatch_remove regex &lt;шаблон&gt;",

		// List
		"list_words_fail":   "Не вдалося отримати список слів.",
		"list_words_empty":  "Немає заблокованих слів. Використайте /tgwatch_add word для додавання.",
		"list_words_header": "<b>Заблоковані слова</b> (%d):\n<code>%s</code>",
		"list_regex_fail":   "Не вдалося отримати regex-шаблони.",
		"list_regex_empty":  "Немає regex-шаблонів. Використайте /tgwatch_add regex для додавання.",
		"list_regex_header": "<b>Regex-шаблони</b> (%d):\n%s",
		"list_usage":        "Використання: /tgwatch_list words  або  /tgwatch_list regex",

		// Clear command buttons
		"btn_cancel":         "Скасувати",
		"btn_clear_words":    "Так, очистити всі слова",
		"btn_clear_regex":    "Так, очистити всі шаблони",
		"btn_clear_log":      "Так, очистити журнал",
		"clear_words_prompt": "Це видалить <b>усі заблоковані слова</b> у цьому чаті.",
		"clear_regex_prompt": "Це видалить <b>усі regex-шаблони</b> у цьому чаті.",
		"clear_usage":        "Використання: /tgwatch_clear words  або  /tgwatch_clear regex",

		// Set
		"set_usage": "Використання: /tgwatch_set &lt;action|mute_duration|sandbox|sandbox_duration|name_filter|verification|verification_timeout&gt; &lt;значення&gt;",
		"set_action_valid": "Допустимі дії:\n<code>delete</code> — тільки видалити повідомлення\n<code>mute</code> — видалити + замовчати на N годин\n<code>kick</code> — видалити + виключити (може повернутись)\n<code>ban</code> — видалити + заблокувати назавжди",
		"set_action_fail":          "Не вдалося оновити дію.",
		"set_action_ok":            "Дію для спаму встановлено: <b>%s</b>.",
		"set_mute_invalid":         "Тривалість заглушення має бути додатнім цілим числом годин.",
		"set_mute_fail":            "Не вдалося оновити тривалість заглушення.",
		"set_mute_ok":              "Тривалість заглушення: <b>%d год.</b>.",
		"set_sandbox_enable_fail":  "Не вдалося увімкнути пісочницю.",
		"set_sandbox_enabled":      "Пісочницю увімкнено. Нові учасники будуть обмежені до закінчення терміну.",
		"set_sandbox_disable_fail": "Не вдалося вимкнути пісочницю.",
		"set_sandbox_disabled":     "Пісочницю вимкнено. Нові учасники можуть писати одразу.",
		"set_sandbox_usage":        "Використання: /tgwatch_set sandbox on|off",
		"set_sandbox_dur_invalid":  "Тривалість пісочниці має бути додатнім цілим числом годин.",
		"set_sandbox_dur_fail":     "Не вдалося оновити тривалість пісочниці.",
		"set_sandbox_dur_ok":       "Тривалість пісочниці: <b>%d год.</b>.",
		"set_nf_enable_fail":       "Не вдалося увімкнути фільтр імен.",
		"set_nf_enabled":           "Фільтр імен увімкнено. Нові учасники зі спам-іменами будуть виключені при вступі.",
		"set_nf_disable_fail":      "Не вдалося вимкнути фільтр імен.",
		"set_nf_disabled":          "Фільтр імен вимкнено.",
		"set_nf_usage":             "Використання: /tgwatch_set name_filter on|off",
		"set_verif_enable_fail":    "Не вдалося увімкнути верифікацію.",
		"set_verif_enabled":        "Верифікацію увімкнено. Нові учасники повинні натиснути кнопку, щоб писати.",
		"set_verif_disable_fail":   "Не вдалося вимкнути верифікацію.",
		"set_verif_disabled":       "Верифікацію вимкнено. Нові учасники можуть писати одразу (з урахуванням пісочниці).",
		"set_verif_usage":          "Використання: /tgwatch_set verification on|off",
		"set_verif_to_invalid":     "Таймаут верифікації має бути додатнім цілим числом хвилин.",
		"set_verif_to_fail":        "Не вдалося оновити таймаут верифікації.",
		"set_verif_to_ok":          "Таймаут верифікації: <b>%d хв.</b>.",
		"set_unknown":              "Невідоме налаштування. Допустимі ключі:\n<code>action</code>, <code>mute_duration</code>, <code>sandbox</code>, <code>sandbox_duration</code>, <code>name_filter</code>, <code>verification</code>, <code>verification_timeout</code>",

		// Show settings
		"show_settings_fail": "Помилка отримання налаштувань.",
		"show_settings": "<b>Налаштування цього чату</b>\n\n" +
			"Дія для спаму:         <b>%s</b>\n" +
			"Тривалість заглушення: <b>%dh</b>\n" +
			"Фільтр імен:           <b>%s</b>\n" +
			"Верифікація:           <b>%s</b>\n" +
			"Таймаут верифікації:   <b>%dm</b>\n" +
			"Пісочниця:             <b>%s</b>\n" +
			"Тривалість пісочниці:  <b>%dh</b>\n" +
			"Мова:                  <b>%s</b>",
		"settings_on":     "увімк.",
		"settings_off":    "вимк.",
		"settings_lang_en": "🇬🇧 English",
		"settings_lang_uk": "🇺🇦 Українська",

		// Check [FEAT-008]
		"check_usage":    "Використання: /tgwatch_check &lt;текст повідомлення&gt;",
		"check_fail":     "Помилка перевірки.",
		"check_cfg_fail": "Помилка отримання налаштувань.",
		"check_no_match": "Збігів немає — це повідомлення пройде.",
		"check_match": "<b>Збіг знайдено</b>\n" +
			"Тип:     <b>%s</b>\n" +
			"Шаблон: <code>%s</code>\n" +
			"Нормалізований ввід: <code>%s</code>\n" +
			"Буде дія: <b>%s</b>",

		// Unrestrict [FEAT-007]
		"unrestrict_usage": "Дайте відповідь на повідомлення користувача, щоб зняти обмеження.",
		"unrestrict_ok":    "Обмеження користувача <b>#%d</b> знято, він може писати знову.",

		// Log [FEAT-009]
		"log_clear_prompt": "Це очистить <b>весь журнал спаму</b> цього чату.",
		"log_usage":        "Використання: /tgwatch_log [n|clear]",
		"log_fail":         "Помилка отримання журналу спаму.",
		"log_empty":        "Журнал спаму порожній.",
		"log_header":       "<b>Останні %d записів спаму:</b>\n\n%s",

		// Copy [FEAT-012]
		"copy_usage":       "Використання: /tgwatch_copy &lt;chat_id_джерела&gt; &lt;chat_id_цілі&gt;",
		"copy_invalid_ids": "ID чатів мають бути числовими.",
		"copy_prompt": "Скопіювати всі налаштування з <code>%d</code> до <code>%d</code>?\n" +
			"<i>Це замінить усі слова, шаблони та налаштування у цільовому чаті.</i>\n" +
			"Напишіть <b>YES</b> для підтвердження (60 сек).",

		// Lang command [FEAT-016]
		"lang_prompt": "Оберіть мову бота для цього чату:",
		"btn_lang_en": "🇬🇧 English",
		"btn_lang_uk": "🇺🇦 Українська",
		"lang_set_en": "Мову встановлено на 🇬🇧 Англійська.",
		"lang_set_uk": "Мову встановлено на 🇺🇦 Українська.",
		"lang_fail":   "Не вдалося зберегти мову.",

		// Help
		"help": `<b>Команди tgwatchspam</b>

<b>Фільтри</b>
/tgwatch_add word &lt;текст&gt; — додати заблоковане слово/фразу
/tgwatch_add regex &lt;шаблон&gt; — додати RE2 regex-шаблон
/tgwatch_remove word &lt;текст&gt; — видалити слово
/tgwatch_remove regex &lt;шаблон&gt; — видалити шаблон
/tgwatch_list words — показати заблоковані слова
/tgwatch_list regex — показати regex-шаблони
/tgwatch_clear words — видалити всі слова
/tgwatch_clear regex — видалити всі шаблони

<b>Дії</b>
/tgwatch_set action delete — тільки видалити повідомлення
/tgwatch_set action mute — видалити + замовчати на N годин
/tgwatch_set action kick — видалити + виключити (може повернутись)
/tgwatch_set action ban — видалити + заблокувати назавжди
/tgwatch_set mute_duration &lt;годин&gt; — тривалість заглушення (за замовч. 24г)

<b>Контроль нових учасників</b>
/tgwatch_set sandbox on|off
/tgwatch_set sandbox_duration &lt;годин&gt;
/tgwatch_set verification on|off
/tgwatch_set verification_timeout &lt;хвилин&gt;
/tgwatch_set name_filter on|off
/tgwatch_unrestrict — відповісти на повідомлення, щоб зняти обмеження

<b>Управління</b>
/tgwatch_show settings — поточні налаштування
/tgwatch_check &lt;текст&gt; — перевірити текст без дії
/tgwatch_log [n] — показати останні N записів спаму
/tgwatch_log clear — очистити журнал спаму
/tgwatch_clean — видалити всі відповіді бота та команди адміністратора
/tgwatch_lang — змінити мову бота (🇬🇧 / 🇺🇦)`,
	},
}
