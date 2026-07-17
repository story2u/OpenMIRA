import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Platform,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';

import { migrateInstalledToken } from './src/auth/migrateInstalledToken';
import { runPiHermesSpike } from './src/spikes/piHermes';
import { runSQLiteRecoverySpike } from './src/spikes/sqliteRecovery';
import { runNativeStreamingSpike } from './src/spikes/streaming';

type Check = 'pi' | 'sqlite' | 'stream' | 'token';

const checks: Array<{ key: Check; label: string }> = [
  { key: 'pi', label: 'pi stream + tool call' },
  { key: 'sqlite', label: 'SQLite 10k recovery' },
  { key: 'stream', label: 'UTF-8 SSE stream' },
  { key: 'token', label: 'Legacy token migration' },
];

export default function App() {
  const [running, setRunning] = useState<Check | null>(null);
  const [results, setResults] = useState<Partial<Record<Check, string>>>({});

  async function run(check: Check) {
    setRunning(check);
    try {
      const result =
        check === 'pi'
          ? await runPiHermesSpike()
          : check === 'sqlite'
            ? await runSQLiteRecoverySpike()
            : check === 'stream'
              ? await runNativeStreamingSpike(
                  Platform.OS === 'android' ? 'http://10.0.2.2:8787' : 'http://127.0.0.1:8787',
                )
              : await migrateInstalledToken();
      setResults((current) => ({ ...current, [check]: JSON.stringify(result) }));
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setResults((current) => ({ ...current, [check]: `ERROR: ${message}` }));
    } finally {
      setRunning(null);
    }
  }

  useEffect(() => {
    void (async () => {
      for (const check of checks) await run(check.key);
    })();
  }, []);

  return (
    <SafeAreaView style={styles.safeArea}>
      <ScrollView contentContainerStyle={styles.container}>
        <Text style={styles.eyebrow}>AGENT-NATIVE / P0</Text>
        <Text style={styles.title}>商机雷达兼容性实验室</Text>
        <Text style={styles.description}>
          这里运行固定输入，不连接生产 API，也不修改服务端数据。开发包与生产 App 使用不同包名。
        </Text>

        <View style={styles.card}>
          {checks.map((check) => (
            <View key={check.key} style={styles.row}>
              <Pressable
                accessibilityRole="button"
                disabled={running !== null}
                onPress={() => run(check.key)}
                style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}
              >
                <Text style={styles.buttonText}>{check.label}</Text>
              </Pressable>
              {running === check.key ? (
                <ActivityIndicator color="#2dd4bf" />
              ) : (
                <Text selectable style={styles.result}>
                  {results[check.key] ?? '尚未运行'}
                </Text>
              )}
            </View>
          ))}
        </View>
        <StatusBar style="light" />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1, backgroundColor: '#07111f' },
  container: { flexGrow: 1, padding: 24, gap: 14 },
  eyebrow: { marginTop: 24, color: '#2dd4bf', fontSize: 13, fontWeight: '700', letterSpacing: 2 },
  title: { color: '#f8fafc', fontSize: 30, fontWeight: '800' },
  description: { color: '#94a3b8', fontSize: 15, lineHeight: 23 },
  card: { marginTop: 14, gap: 14 },
  row: { padding: 16, gap: 12, borderRadius: 16, backgroundColor: '#0f2035' },
  button: { alignSelf: 'flex-start', paddingHorizontal: 14, paddingVertical: 10, borderRadius: 10, backgroundColor: '#164e63' },
  buttonPressed: { opacity: 0.72 },
  buttonText: { color: '#ecfeff', fontSize: 15, fontWeight: '700' },
  result: { color: '#cbd5e1', fontFamily: 'monospace', fontSize: 12, lineHeight: 18 },
});
