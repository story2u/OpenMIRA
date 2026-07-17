import { StyleSheet, View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, { useAnimatedStyle, useSharedValue } from 'react-native-reanimated';
import Svg, { Circle, G, Line, Text as SvgText } from 'react-native-svg';

import type { Translator } from '../../i18n/core';
import type { MessageKey } from '../../i18n/catalog';
import type { IntentMapModel, IntentMapNode } from './intent-map-model';

function nodeColors(kind: IntentMapNode['kind']) {
  switch (kind) {
    case 'self': return { fill: '#0f766e', stroke: '#5eead4', text: '#f0fdfa' };
    case 'reduce': return { fill: '#342632', stroke: '#9f6479', text: '#fecdd3' };
    case 'temporary': return { fill: '#3b2562', stroke: '#a78bfa', text: '#ede9fe' };
    case 'context': return { fill: '#172d45', stroke: '#5b7895', text: '#dbeafe' };
    case 'core': return { fill: '#12344b', stroke: '#38bdf8', text: '#e0f2fe' };
  }
}

const localizedConcepts = new Set([
  'important_customer', 'purchase_intent', 'needs_reply', 'suitable_job',
  'current_project', 'deadline', 'industry_signal', 'advertising', 'training',
  'duplicate', 'unrelated_chat', 'expired', 'untrusted_source',
]);

export function displayConcept(value: string, t: Translator) {
  if (value === 'self') return t('intentMap.self');
  if (localizedConcepts.has(value)) return t(`teaching.reason.${value}` as MessageKey);
  return value.replaceAll('_', ' ');
}

export function IntentMapCanvas({
  activeIntentIds,
  compact = false,
  model,
  onNodePress,
  t,
}: {
  activeIntentIds?: ReadonlySet<string> | null;
  compact?: boolean;
  model: IntentMapModel;
  onNodePress(node: IntentMapNode): void;
  t: Translator;
}) {
  const x = useSharedValue(0);
  const y = useSharedValue(0);
  const startX = useSharedValue(0);
  const startY = useSharedValue(0);
  const scale = useSharedValue(1);
  const startScale = useSharedValue(1);
  const pan = Gesture.Pan()
    .maxPointers(1)
    .onBegin(() => { startX.value = x.value; startY.value = y.value; })
    .onUpdate((event) => {
      x.value = startX.value + event.translationX;
      y.value = startY.value + event.translationY;
    });
  const pinch = Gesture.Pinch()
    .onBegin(() => { startScale.value = scale.value; })
    .onUpdate((event) => { scale.value = Math.min(1.8, Math.max(0.85, startScale.value * event.scale)); });
  const gesture = Gesture.Simultaneous(pan, pinch);
  const animated = useAnimatedStyle(() => ({
    transform: [{ translateX: x.value }, { translateY: y.value }, { scale: scale.value }],
  }));
  const nodesById = new Map(model.nodes.map((node) => [node.id, node]));
  return (
    <View style={[styles.viewport, compact && styles.compactViewport]}>
      <GestureDetector gesture={gesture}>
        <Animated.View style={[styles.canvas, animated]}>
          <Svg accessibilityElementsHidden height="100%" viewBox="0 0 360 370" width="100%">
            {model.edges.map((edge) => {
              const from = nodesById.get(edge.from);
              const to = nodesById.get(edge.to);
              if (!from || !to) return null;
              return (
                <Line
                  key={edge.id}
                  stroke={to.kind === 'reduce' ? '#6b4454' : '#31536d'}
                  strokeDasharray={edge.confirmed ? undefined : '5 7'}
                  strokeLinecap="round"
                  strokeOpacity={edge.confirmed ? 0.72 : 0.42}
                  strokeWidth={Math.max(1, edge.strength * 2)}
                  x1={from.x}
                  x2={to.x}
                  y1={from.y}
                  y2={to.y}
                />
              );
            })}
            {model.nodes.map((node) => {
              const palette = nodeColors(node.kind);
              const label = displayConcept(node.label, t);
              const timeActive = !activeIntentIds || node.kind === 'self' || node.kind === 'temporary' || activeIntentIds.has(node.id);
              return (
                <G key={node.id} onPress={() => onNodePress(node)} opacity={timeActive ? 1 : 0.28}>
                  <Circle
                    cx={node.x}
                    cy={node.y}
                    fill={palette.fill}
                    r={node.radius + (timeActive && activeIntentIds ? 7 : 5)}
                    stroke={palette.stroke}
                    strokeDasharray={node.confirmed ? undefined : '4 4'}
                    strokeOpacity={0.25 + node.confidence * 0.75}
                    strokeWidth={2.5}
                  />
                  <Circle cx={node.x} cy={node.y} fill={palette.fill} r={node.radius} />
                  <SvgText
                    fill={palette.text}
                    fontSize={node.kind === 'self' ? 11 : 9}
                    fontWeight="700"
                    textAnchor="middle"
                    x={node.x}
                    y={node.y + 3}
                  >
                    {label.length > 15 ? `${label.slice(0, 13)}…` : label}
                  </SvgText>
                  {node.badge ? (
                    <Circle cx={node.x + node.radius * 0.72} cy={node.y - node.radius * 0.72} fill="#8b5cf6" r={5} />
                  ) : null}
                </G>
              );
            })}
          </Svg>
        </Animated.View>
      </GestureDetector>
    </View>
  );
}

const styles = StyleSheet.create({
  viewport: { height: 390, overflow: 'hidden', borderRadius: 24, backgroundColor: '#091827' },
  compactViewport: { height: 290 },
  canvas: { width: '100%', height: '100%' },
});
