import type { DetectionSettings } from '@story2u/radar-contracts/settings';
import { useState } from 'react';
import { Pressable, StyleSheet, Switch, Text, TextInput, View } from 'react-native';

import { useI18n } from '../../i18n/I18nProvider';
import { colors } from '../../ui/theme';
import {
  ActionButton,
  SettingsCard,
  SettingsScaffold,
  StateNotice,
  settingsStyles,
} from './components';
import { useSettings } from './SettingsProvider';

export function DetectionSettingsScreen() {
  const settings = useSettings();
  const { t } = useI18n();
  const initial = settings.state.bundle?.detection;
  if (!initial) {
    return (
      <SettingsScaffold title={t('settings.detection.title')}>
        <StateNotice
          error={settings.state.loadError}
          loading={settings.state.isLoading}
          onRetry={settings.retry}
        />
      </SettingsScaffold>
    );
  }
  return (
    <DetectionEditor
      initial={initial}
      key={`${initial.aiSemanticsEnabled}:${initial.keywords.join('\u0000')}`}
    />
  );
}

function DetectionEditor({ initial }: { initial: DetectionSettings }) {
  const { saveDetection, state } = useSettings();
  const { t } = useI18n();
  const [keywords, setKeywords] = useState(initial.keywords);
  const [aiSemanticsEnabled, setAiSemanticsEnabled] = useState(initial.aiSemanticsEnabled);
  const [newKeyword, setNewKeyword] = useState('');
  const [localError, setLocalError] = useState('');
  const [showSaveError, setShowSaveError] = useState(false);
  const saving = state.busyAction === 'detection';

  function addKeyword() {
    const keyword = newKeyword.trim();
    if (!keyword) return;
    if (keyword.length > 64) {
      setLocalError(t('settings.detection.error.keywordLength'));
      return;
    }
    if (keywords.some((item) => item.toLocaleLowerCase() === keyword.toLocaleLowerCase())) {
      setNewKeyword('');
      return;
    }
    if (keywords.length >= 200) {
      setLocalError(t('settings.detection.error.keywordCount'));
      return;
    }
    setKeywords((current) => [...current, keyword]);
    setNewKeyword('');
    setLocalError('');
  }

  async function save() {
    setShowSaveError(false);
    const saved = await saveDetection({ keywords, aiSemanticsEnabled });
    if (!saved) {
      setKeywords(initial.keywords);
      setAiSemanticsEnabled(initial.aiSemanticsEnabled);
      setShowSaveError(true);
    }
  }

  return (
    <SettingsScaffold
      title={t('settings.detection.title')}
      subtitle={t('settings.detection.subtitle')}
    >
      <SettingsCard>
        <View style={settingsStyles.rowBetween}>
          <View style={{ flex: 1, gap: 4 }}>
            <Text style={settingsStyles.label}>{t('settings.detection.ai')}</Text>
            <Text style={settingsStyles.body}>{t('settings.detection.aiDetail')}</Text>
          </View>
          <Switch
            accessibilityLabel={t('settings.detection.ai')}
            onValueChange={setAiSemanticsEnabled}
            trackColor={{ false: colors.border, true: colors.button }}
            value={aiSemanticsEnabled}
          />
        </View>
      </SettingsCard>

      <SettingsCard>
        <Text style={settingsStyles.label}>{t('settings.detection.keywords')}</Text>
        {keywords.length === 0 ? <Text style={settingsStyles.body}>{t('settings.detection.empty')}</Text> : null}
        <View style={styles.chips}>
          {keywords.map((keyword) => (
            <View key={keyword} style={styles.chip}>
              <Text style={styles.chipText}>{keyword}</Text>
              <Pressable
                accessibilityLabel={t('settings.detection.removeKeyword', { keyword })}
                accessibilityRole="button"
                hitSlop={8}
                onPress={() => setKeywords((current) => current.filter((item) => item !== keyword))}
              >
                <Text style={styles.remove}>×</Text>
              </Pressable>
            </View>
          ))}
        </View>
        <View style={settingsStyles.row}>
          <TextInput
            accessibilityLabel={t('settings.detection.newKeyword')}
            maxLength={65}
            onChangeText={setNewKeyword}
            onSubmitEditing={addKeyword}
            placeholder={t('settings.detection.keywordPlaceholder')}
            placeholderTextColor={colors.placeholder}
            style={[settingsStyles.input, { flex: 1 }]}
            value={newKeyword}
          />
          <ActionButton disabled={!newKeyword.trim()} label={t('common.add')} onPress={addKeyword} tone="secondary" />
        </View>
      </SettingsCard>

      {localError ? <Text accessibilityRole="alert" style={settingsStyles.error}>{localError}</Text> : null}
      {showSaveError && state.saveError ? (
        <Text accessibilityRole="alert" style={settingsStyles.error}>{state.saveError}</Text>
      ) : null}
      <ActionButton busy={saving} label={saving ? t('common.saving') : t('settings.detection.save')} onPress={() => void save()} />
    </SettingsScaffold>
  );
}

const styles = StyleSheet.create({
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 7,
    borderRadius: 10,
    backgroundColor: colors.accentMuted,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  chipText: { color: colors.text, fontWeight: '700' },
  remove: { color: colors.mutedText, fontSize: 20, lineHeight: 20 },
});
