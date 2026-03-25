package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	aipkg "perfolizer/pkg/ai"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const prefAISettingsKey = "aiSettings"

func settingsSections() []string {
	return []string{"General", "AI", "Shortcuts", "Agents"}
}

func shouldOpenAIPanelOnStartup(settings aipkg.AISettings) bool {
	normalized := settings.Normalize()
	return normalized.Enabled && normalized.IsConfigured()
}

func (pa *PerfolizerApp) loadAISettings() aipkg.AISettings {
	raw := strings.TrimSpace(pa.FyneApp.Preferences().StringWithFallback(prefAISettingsKey, ""))
	settings := aipkg.DefaultSettings()
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			settings = aipkg.DefaultSettings()
		}
	}
	if pa.secretStore != nil {
		apiKey, err := pa.secretStore.Get(openAIAPIKeySecretKey)
		switch {
		case err == nil:
			settings.APIKey = apiKey
		case errors.Is(err, errSecretNotFound), errors.Is(err, errSecretUnsupported):
		default:
			settings.APIKey = ""
		}
	}
	return settings.Normalize()
}

func (pa *PerfolizerApp) saveAISettings(settings aipkg.AISettings) error {
	settings = settings.Normalize()
	if err := pa.persistOpenAIAPIKey(settings.APIKey); err != nil {
		return err
	}
	payload, err := json.Marshal(scrubAISettingsForPreferences(settings))
	if err == nil {
		pa.FyneApp.Preferences().SetString(prefAISettingsKey, string(payload))
	}
	pa.applyAISettings(settings)
	return nil
}

func (pa *PerfolizerApp) applyAISettings(settings aipkg.AISettings) {
	pa.AISettings = settings.Normalize()
	pa.aiEngine = aipkg.NewEngine(pa.AISettings)
	if !pa.isAIAvailable() {
		pa.aiPanelVisible = false
		pa.clearAIResult()
	}
	pa.updateAIPanelState()
}

func (pa *PerfolizerApp) isAIAvailable() bool {
	return pa.AISettings.Enabled && pa.AISettings.IsConfigured()
}

func scrubAISettingsForPreferences(settings aipkg.AISettings) aipkg.AISettings {
	scrubbed := settings.Normalize()
	scrubbed.APIKey = ""
	return scrubbed
}

func (pa *PerfolizerApp) persistOpenAIAPIKey(apiKey string) error {
	if pa.secretStore == nil {
		return fmt.Errorf("%w: no secure storage backend", errSecretUnsupported)
	}
	if strings.TrimSpace(apiKey) == "" {
		err := pa.secretStore.Delete(openAIAPIKeySecretKey)
		if errors.Is(err, errSecretNotFound) {
			return nil
		}
		return err
	}
	return pa.secretStore.Set(openAIAPIKeySecretKey, apiKey)
}

func (pa *PerfolizerApp) showPreferencesWithSection(initialSection string) {
	if pa.settingsWindow != nil {
		if strings.TrimSpace(initialSection) == "" || initialSection == "General" {
			pa.settingsWindow.RequestFocus()
			return
		}
		pa.settingsWindow.Close()
	}

	w := pa.FyneApp.NewWindow("Settings")
	w.Resize(fyne.NewSize(1320, 820))
	pa.settingsWindow = w
	w.SetOnClosed(func() {
		pa.settingsWindow = nil
	})

	sections := settingsSections()
	content := container.NewMax()

	shortcutsPage := pa.buildShortcutsPage()
	aiPage := pa.buildAISettingsPage(w)
	agentsPage := pa.buildAgentsPage(w)
	pages := map[string]fyne.CanvasObject{
		"General":   container.NewPadded(widget.NewCard("General", "", widget.NewLabel("General settings will appear here."))),
		"AI":        aiPage,
		"Shortcuts": shortcutsPage,
		"Agents":    agentsPage,
	}

	setSection := func(name string) {
		page, ok := pages[name]
		if !ok {
			return
		}
		content.Objects = []fyne.CanvasObject{page}
		content.Refresh()
	}

	sectionList := widget.NewList(
		func() int { return len(sections) },
		func() fyne.CanvasObject { return widget.NewLabel("Section") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(sections[i])
		},
	)
	sectionList.OnSelected = func(id widget.ListItemID) {
		setSection(sections[id])
	}

	layout := container.NewHSplit(
		container.NewPadded(widget.NewCard("Settings", "", sectionList)),
		container.NewPadded(content),
	)
	layout.SetOffset(0.2)
	w.SetContent(layout)

	target := "General"
	if strings.TrimSpace(initialSection) != "" {
		target = initialSection
	}
	selected := false
	for i, section := range sections {
		if section == target {
			sectionList.Select(i)
			selected = true
			break
		}
	}
	if !selected {
		sectionList.Select(0)
	}
	w.Show()
}

func (pa *PerfolizerApp) buildAISettingsPage(win fyne.Window) fyne.CanvasObject {
	settings := pa.AISettings.Normalize()
	codexAuthOK := settings.CodexAuthOK
	openAIKeyStored := strings.TrimSpace(settings.APIKey) != ""

	enabledCheck := widget.NewCheck("Enable AI-assisted authoring", nil)
	enabledCheck.SetChecked(settings.Enabled)

	providerSelect := widget.NewSelect([]string{"Hybrid", "OpenAI", "Codex", "Local"}, nil)
	providerSelect.SetSelected(providerLabel(settings.Provider))

	defaultModelEntry := widget.NewEntry()
	defaultModelEntry.SetText(settings.DefaultModel)

	heavyModelEntry := widget.NewEntry()
	heavyModelEntry.SetText(settings.HeavyModel)

	codexModelEntry := widget.NewEntry()
	codexModelEntry.SetText(settings.CodexModel)

	localModelEntry := widget.NewEntry()
	localModelEntry.SetText(settings.LocalModel)

	openAIBaseURLEntry := widget.NewEntry()
	openAIBaseURLEntry.SetText(settings.OpenAIBaseURL)
	openAIBaseURLEntry.SetPlaceHolder("https://api.openai.com/v1")

	localBaseURLEntry := widget.NewEntry()
	localBaseURLEntry.SetText(settings.LocalBaseURL)
	localBaseURLEntry.SetPlaceHolder("http://127.0.0.1:11434/v1")

	apiKeyEntry := widget.NewPasswordEntry()
	apiKeyEntry.SetText(settings.APIKey)
	if openAIKeyStored {
		apiKeyEntry.SetPlaceHolder("Stored securely. Edit to replace.")
	} else {
		apiKeyEntry.SetPlaceHolder("OpenAI API key")
	}

	codexCLIPathEntry := widget.NewEntry()
	codexCLIPathEntry.SetText(settings.CodexCLIPath)
	codexCLIPathEntry.SetPlaceHolder("codex")

	codexHomeDirEntry := widget.NewEntry()
	codexHomeDirEntry.SetText(settings.CodexHomeDir)
	codexHomeDirEntry.SetPlaceHolder("Codex home directory")

	redactSecretsCheck := widget.NewCheck("Redact secrets before cloud requests", nil)
	redactSecretsCheck.SetChecked(settings.RedactSecrets)

	requireReviewCheck := widget.NewCheck("Require preview review before applying", nil)
	requireReviewCheck.SetChecked(settings.RequireReview)

	statusLabel := widget.NewLabel(settings.Summary())
	statusLabel.Wrapping = fyne.TextWrapWord
	connectionStatusLabel := widget.NewLabel("")
	connectionStatusLabel.Wrapping = fyne.TextWrapWord

	authURLEntry := NewReadOnlyEntry()
	authURLEntry.SetMinRowsVisible(3)
	if pa.codexAuthSession != nil {
		authURLEntry.SetText(pa.codexAuthSession.AuthURL)
	}

	callbackEntry := widget.NewEntry()
	callbackEntry.SetPlaceHolder("Paste the Codex callback URL here if localhost redirect did not complete automatically")

	codexStatusLabel := widget.NewLabel(pa.codexStatusText(settings, codexAuthOK))
	codexStatusLabel.Wrapping = fyne.TextWrapWord
	secureStorageLabel := widget.NewLabel(pa.secureStorageStatusText())
	secureStorageLabel.Wrapping = fyne.TextWrapWord

	currentSettings := func() aipkg.AISettings {
		return buildAISettingsFromForm(
			enabledCheck.Checked,
			providerSelect.Selected,
			defaultModelEntry.Text,
			heavyModelEntry.Text,
			codexModelEntry.Text,
			localModelEntry.Text,
			openAIBaseURLEntry.Text,
			localBaseURLEntry.Text,
			apiKeyEntry.Text,
			codexCLIPathEntry.Text,
			codexHomeDirEntry.Text,
			codexAuthOK,
			redactSecretsCheck.Checked,
			requireReviewCheck.Checked,
		)
	}

	refreshStatus := func() {
		current := currentSettings()
		statusLabel.SetText(current.Summary())
		codexStatusLabel.SetText(pa.codexStatusText(current, codexAuthOK))
	}

	enabledCheck.OnChanged = func(bool) { refreshStatus() }
	providerSelect.OnChanged = func(string) { refreshStatus() }
	defaultModelEntry.OnChanged = func(string) { refreshStatus() }
	heavyModelEntry.OnChanged = func(string) { refreshStatus() }
	codexModelEntry.OnChanged = func(string) { refreshStatus() }
	localModelEntry.OnChanged = func(string) { refreshStatus() }
	openAIBaseURLEntry.OnChanged = func(string) { refreshStatus() }
	localBaseURLEntry.OnChanged = func(string) { refreshStatus() }
	apiKeyEntry.OnChanged = func(string) { refreshStatus() }
	codexCLIPathEntry.OnChanged = func(string) { refreshStatus() }
	codexHomeDirEntry.OnChanged = func(string) { refreshStatus() }
	redactSecretsCheck.OnChanged = func(bool) { refreshStatus() }
	requireReviewCheck.OnChanged = func(bool) { refreshStatus() }

	var testConnectionButton *widget.Button
	var checkCodexStatusButton *widget.Button

	saveButton := widget.NewButton("Save AI settings", func() {
		settings := currentSettings()
		if err := pa.saveAISettings(settings); err != nil {
			dialog.ShowError(err, win)
			return
		}
		openAIKeyStored = strings.TrimSpace(settings.APIKey) != ""
		refreshStatus()
		dialog.ShowInformation("AI settings", "AI settings saved.", win)
	})

	testConnectionButton = widget.NewButton("Test connection", func() {
		settings := currentSettings()
		testConnectionButton.Disable()
		connectionStatusLabel.SetText("Checking provider connection...")
		pa.runAIConnectionCheck(settings, func() {
			if settings.Provider == aipkg.ProviderCodex {
				codexAuthOK = true
				if err := pa.saveAISettings(currentSettings()); err != nil {
					connectionStatusLabel.SetText("Connection check succeeded, but saving Codex auth state failed: " + err.Error())
					testConnectionButton.Enable()
					return
				}
				refreshStatus()
			}
			connectionStatusLabel.SetText("Provider connection check succeeded.")
			testConnectionButton.Enable()
		}, func(err error) {
			connectionStatusLabel.SetText("Connection check failed: " + err.Error())
			testConnectionButton.Enable()
		})
	})

	removeStoredKeyButton := widget.NewButton("Remove stored key", func() {
		apiKeyEntry.SetText("")
		if err := pa.persistOpenAIAPIKey(""); err != nil {
			dialog.ShowError(err, win)
			return
		}
		openAIKeyStored = false
		settings := currentSettings()
		if err := pa.saveAISettings(settings); err != nil {
			dialog.ShowError(err, win)
			return
		}
		refreshStatus()
		dialog.ShowInformation("AI settings", "Stored OpenAI API key removed.", win)
	})

	resetButton := widget.NewButton("Reset defaults", func() {
		defaults := aipkg.DefaultSettings()
		enabledCheck.SetChecked(defaults.Enabled)
		providerSelect.SetSelected(providerLabel(defaults.Provider))
		defaultModelEntry.SetText(defaults.DefaultModel)
		heavyModelEntry.SetText(defaults.HeavyModel)
		codexModelEntry.SetText(defaults.CodexModel)
		localModelEntry.SetText(defaults.LocalModel)
		openAIBaseURLEntry.SetText(defaults.OpenAIBaseURL)
		localBaseURLEntry.SetText(defaults.LocalBaseURL)
		apiKeyEntry.SetText(defaults.APIKey)
		openAIKeyStored = false
		codexCLIPathEntry.SetText(defaults.CodexCLIPath)
		codexHomeDirEntry.SetText(defaults.CodexHomeDir)
		codexAuthOK = defaults.CodexAuthOK
		redactSecretsCheck.SetChecked(defaults.RedactSecrets)
		requireReviewCheck.SetChecked(defaults.RequireReview)
		authURLEntry.SetText("")
		callbackEntry.SetText("")
		if pa.codexAuthSession != nil {
			_ = pa.codexAuthSession.Cancel()
			pa.codexAuthSession = nil
		}
		refreshStatus()
	})

	openOpenAIDocsButton := widget.NewButton("Open OpenAI docs", func() {
		if err := pa.openExternalURL("https://platform.openai.com/docs/quickstart/getting-started"); err != nil {
			dialog.ShowError(err, win)
		}
	})

	openOpenAIPlatformButton := widget.NewButton("Open OpenAI platform", func() {
		if err := pa.openExternalURL("https://platform.openai.com/"); err != nil {
			dialog.ShowError(err, win)
		}
	})

	startCodexLoginButton := widget.NewButton("Start Codex login", func() {
		settings := currentSettings()
		if pa.codexAuthSession != nil {
			_ = pa.codexAuthSession.Cancel()
			pa.codexAuthSession = nil
		}
		session, err := aipkg.StartCodexLogin(settings)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		pa.codexAuthSession = session
		codexAuthOK = false
		authURLEntry.SetText(session.AuthURL)
		codexStatusLabel.SetText("Codex login started. Open the auth URL, complete authentication in the browser, then paste the callback URL below if the localhost redirect does not finish automatically.")
		refreshStatus()
	})

	openCodexAuthButton := widget.NewButton("Open auth URL", func() {
		if pa.codexAuthSession == nil || strings.TrimSpace(pa.codexAuthSession.AuthURL) == "" {
			dialog.ShowInformation("Codex login", "Start the Codex login flow first.", win)
			return
		}
		if err := pa.openExternalURL(pa.codexAuthSession.AuthURL); err != nil {
			dialog.ShowError(err, win)
		}
	})

	completeCodexCallbackButton := widget.NewButton("Complete callback", func() {
		if pa.codexAuthSession == nil {
			dialog.ShowInformation("Codex login", "Start the Codex login flow first.", win)
			return
		}
		callbackURL := strings.TrimSpace(callbackEntry.Text)
		if callbackURL == "" {
			dialog.ShowError(fmt.Errorf("callback URL is required"), win)
			return
		}

		codexStatusLabel.SetText("Completing Codex login...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			err := pa.codexAuthSession.Complete(ctx, callbackURL)
			fyne.Do(func() {
				if err != nil {
					codexStatusLabel.SetText("Codex login failed.")
					dialog.ShowError(err, win)
					return
				}
				codexAuthOK = true
				if err := pa.saveAISettings(currentSettings()); err != nil {
					dialog.ShowError(err, win)
					return
				}
				if pa.codexAuthSession != nil {
					_ = pa.codexAuthSession.Cancel()
					pa.codexAuthSession = nil
				}
				authURLEntry.SetText("")
				callbackEntry.SetText("")
				codexStatusLabel.SetText("Codex authenticated. The provider is ready to use.")
				refreshStatus()
				dialog.ShowInformation("Codex login", "Codex authentication completed.", win)
			})
		}()
	})

	checkCodexStatusButton = widget.NewButton("Check Codex status", func() {
		settings := currentSettings()
		settings.Provider = aipkg.ProviderCodex
		checkCodexStatusButton.Disable()
		connectionStatusLabel.SetText("Checking Codex status...")
		pa.runAIConnectionCheck(settings, func() {
			codexAuthOK = true
			if err := pa.saveAISettings(currentSettings()); err != nil {
				connectionStatusLabel.SetText("Codex status check succeeded, but saving auth state failed: " + err.Error())
				checkCodexStatusButton.Enable()
				return
			}
			refreshStatus()
			connectionStatusLabel.SetText("Codex status check succeeded.")
			checkCodexStatusButton.Enable()
		}, func(err error) {
			connectionStatusLabel.SetText("Codex status check failed: " + err.Error())
			checkCodexStatusButton.Enable()
		})
	})

	form := widget.NewForm(
		widget.NewFormItem("", enabledCheck),
		widget.NewFormItem("Provider", providerSelect),
		widget.NewFormItem("Default model", defaultModelEntry),
		widget.NewFormItem("Heavy model", heavyModelEntry),
		widget.NewFormItem("Codex model", codexModelEntry),
		widget.NewFormItem("Local model", localModelEntry),
		widget.NewFormItem("OpenAI base URL", openAIBaseURLEntry),
		widget.NewFormItem("OpenAI API key", apiKeyEntry),
		widget.NewFormItem("Local base URL", localBaseURLEntry),
		widget.NewFormItem("Codex CLI path", codexCLIPathEntry),
		widget.NewFormItem("Codex home dir", codexHomeDirEntry),
		widget.NewFormItem("", redactSecretsCheck),
		widget.NewFormItem("", requireReviewCheck),
	)

	openAIHelp := widget.NewCard("OpenAI setup", "", container.NewVBox(
		widget.NewLabel("OpenAI uses API key auth for the normal API flow."),
		secureStorageLabel,
		widget.NewLabel("1. Open the OpenAI platform dashboard."),
		widget.NewLabel("2. Create a project API key there."),
		widget.NewLabel("3. Paste it into “OpenAI API key”."),
		widget.NewLabel("4. Perfolizer stores the key in secure OS secret storage, not in normal app preferences."),
		widget.NewLabel("5. Use “Test connection” before enabling the provider."),
		container.NewHBox(openOpenAIDocsButton, openOpenAIPlatformButton, removeStoredKeyButton),
	))

	codexHelp := widget.NewCard("Codex setup", "", container.NewVBox(
		widget.NewLabel("Codex can be used as a separate provider through the local Codex CLI."),
		widget.NewLabel("1. Start the Codex login flow to generate an auth URL."),
		widget.NewLabel("2. Open that URL in a browser and authenticate."),
		widget.NewLabel("3. If localhost redirect does not complete automatically, paste the callback URL below."),
		codexStatusLabel,
		widget.NewForm(
			widget.NewFormItem("Auth URL", authURLEntry),
			widget.NewFormItem("Callback URL", callbackEntry),
		),
		container.NewHBox(startCodexLoginButton, openCodexAuthButton, completeCodexCallbackButton, checkCodexStatusButton),
	))

	content := container.NewVBox(
		widget.NewLabel("Configure optional AI-assisted authoring. The editor remains fully usable without any provider."),
		statusLabel,
		connectionStatusLabel,
		form,
		container.NewHBox(saveButton, testConnectionButton, resetButton),
		openAIHelp,
		codexHelp,
	)
	scroll := container.NewVScroll(container.NewPadded(widget.NewCard("AI", "", content)))
	scroll.SetMinSize(fyne.NewSize(0, 640))
	return scroll
}

func buildAISettingsFromForm(enabled bool, providerLabel, defaultModel, heavyModel, codexModel, localModel, openAIBaseURL, localBaseURL, apiKey, codexCLIPath, codexHomeDir string, codexAuthOK, redactSecrets, requireReview bool) aipkg.AISettings {
	settings := aipkg.DefaultSettings()
	settings.Enabled = enabled
	settings.Provider = providerType(providerLabel)
	settings.DefaultModel = defaultModel
	settings.HeavyModel = heavyModel
	settings.CodexModel = codexModel
	settings.LocalModel = localModel
	settings.OpenAIBaseURL = openAIBaseURL
	settings.LocalBaseURL = localBaseURL
	settings.APIKey = apiKey
	settings.CodexCLIPath = codexCLIPath
	settings.CodexHomeDir = codexHomeDir
	settings.CodexAuthOK = codexAuthOK
	settings.RedactSecrets = redactSecrets
	settings.RequireReview = requireReview
	return settings.Normalize()
}

func providerLabel(provider aipkg.ProviderType) string {
	switch provider {
	case aipkg.ProviderOpenAI:
		return "OpenAI"
	case aipkg.ProviderLocal:
		return "Local"
	case aipkg.ProviderHybrid:
		return "Hybrid"
	case aipkg.ProviderCodex:
		return "Codex"
	default:
		return "Hybrid"
	}
}

func providerType(label string) aipkg.ProviderType {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "openai":
		return aipkg.ProviderOpenAI
	case "codex":
		return aipkg.ProviderCodex
	case "local":
		return aipkg.ProviderLocal
	default:
		return aipkg.ProviderHybrid
	}
}

func (pa *PerfolizerApp) openExternalURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	return pa.FyneApp.OpenURL(parsed)
}

func (pa *PerfolizerApp) runAIConnectionCheck(settings aipkg.AISettings, onSuccess func(), onError func(error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		engine := aipkg.NewEngine(settings)
		err := engine.TestConnection(ctx)
		fyne.Do(func() {
			if err != nil {
				if onError != nil {
					onError(err)
				}
				return
			}
			if onSuccess != nil {
				onSuccess()
			}
		})
	}()
}

func (pa *PerfolizerApp) codexStatusText(settings aipkg.AISettings, codexAuthOK bool) string {
	settings = settings.Normalize()
	switch {
	case strings.TrimSpace(settings.CodexCLIPath) == "":
		return "Codex status: CLI path is empty."
	case strings.TrimSpace(settings.CodexHomeDir) == "":
		return "Codex status: home directory is empty."
	case pa.codexAuthSession != nil && strings.TrimSpace(pa.codexAuthSession.AuthURL) != "":
		return "Codex status: login in progress."
	case codexAuthOK:
		return "Codex status: authenticated."
	default:
		return "Codex status: not authenticated yet."
	}
}

func (pa *PerfolizerApp) secureStorageStatusText() string {
	if pa.secretStore == nil {
		return "Secure storage backend: unavailable."
	}
	return fmt.Sprintf("Secure storage backend: %s.", pa.secretStore.BackendName())
}
