import * as Haptics from 'expo-haptics';
import { forwardRef, useImperativeHandle } from 'react';
import { StyleSheet, Text, View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  Easing,
  Extrapolation,
  interpolate,
  runOnJS,
  useAnimatedStyle,
  useSharedValue,
  withSpring,
  withTiming,
} from 'react-native-reanimated';

import type { TeachingMessageCard as TeachingCard } from '../../attention/teachingService';
import type { Translator } from '../../i18n/core';
import { colors } from '../../ui/theme';
import { TeachingMessageCard } from './TeachingMessageCard';

export interface TeachingCardDeckHandle {
  commit(label: 'positive' | 'negative'): void;
}

interface Props {
  card: TeachingCard;
  compact: boolean;
  nextCard: TeachingCard | null;
  onCommit(label: 'positive' | 'negative'): void;
  onDragging(direction: 'left' | 'right' | null): void;
  reduceMotion: boolean;
  t: Translator;
}

function haptic(label: 'positive' | 'negative') {
  if (label === 'positive') {
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  } else {
    void Haptics.selectionAsync();
  }
}

export const TeachingCardDeck = forwardRef<TeachingCardDeckHandle, Props>(
  function TeachingCardDeck({ card, compact, nextCard, onCommit, onDragging, reduceMotion, t }, ref) {
    const translateX = useSharedValue(0);
    const width = useSharedValue(320);
    const direction = useSharedValue(0);
    const thresholdCrossed = useSharedValue(0);
    const committed = useSharedValue(0);
    const startX = useSharedValue(100);

    const finish = (label: 'positive' | 'negative') => onCommit(label);
    const announceDirection = (value: number) => {
      onDragging(value < 0 ? 'left' : value > 0 ? 'right' : null);
    };

    const animateCommit = (label: 'positive' | 'negative') => {
      committed.value = 1;
      haptic(label);
      const destination = (label === 'positive' ? -1 : 1) * (width.value + 80);
      translateX.value = withTiming(destination, {
        duration: reduceMotion ? 120 : 220,
        easing: Easing.bezier(0.22, 1, 0.36, 1),
      }, (finished) => {
        if (finished) runOnJS(finish)(label);
      });
    };

    useImperativeHandle(ref, () => ({ commit: animateCommit }));

    const pan = Gesture.Pan()
      .maxPointers(1)
      .activeOffsetX([-12, 12])
      .failOffsetY([-18, 18])
      .hitSlop({ left: -24 })
      .onBegin((event) => {
        startX.value = event.absoluteX;
        committed.value = 0;
        thresholdCrossed.value = 0;
        direction.value = 0;
      })
      .onUpdate((event) => {
        if (startX.value < 24) return;
        translateX.value = event.translationX;
        const nextDirection = event.translationX < -2 ? -1 : event.translationX > 2 ? 1 : 0;
        if (nextDirection !== direction.value) {
          direction.value = nextDirection;
          runOnJS(announceDirection)(nextDirection);
        }
        const crossed = Math.abs(event.translationX) >= width.value * 0.4
          ? nextDirection
          : 0;
        if (crossed !== 0 && thresholdCrossed.value !== crossed) {
          thresholdCrossed.value = crossed;
          runOnJS(haptic)(crossed < 0 ? 'positive' : 'negative');
        } else if (crossed === 0) {
          thresholdCrossed.value = 0;
        }
      })
      .onEnd((event) => {
        if (startX.value < 24) return;
        const horizontal = Math.abs(event.translationX);
        const dominant = horizontal > Math.abs(event.translationY) * 1.25;
        const distance = horizontal >= width.value * 0.4;
        const fling = horizontal >= width.value * 0.08 && Math.abs(event.velocityX) >= 850;
        if (dominant && (distance || fling)) {
          committed.value = 1;
          const label = event.translationX < 0 ? 'positive' : 'negative';
          if (thresholdCrossed.value === 0) runOnJS(haptic)(label);
          const destination = (label === 'positive' ? -1 : 1) * (width.value + 80);
          translateX.value = withTiming(destination, {
            duration: reduceMotion ? 120 : 220,
            easing: Easing.bezier(0.22, 1, 0.36, 1),
          }, (finished) => {
            if (finished) runOnJS(finish)(label);
          });
          return;
        }
        translateX.value = withSpring(0, {
          damping: 24,
          stiffness: 260,
          overshootClamping: true,
        });
      })
      .onFinalize(() => {
        if (!committed.value) {
          direction.value = 0;
          runOnJS(announceDirection)(0);
        }
      });

    const cardStyle = useAnimatedStyle(() => ({
      opacity: reduceMotion
        ? interpolate(Math.abs(translateX.value), [0, width.value * 0.4], [1, 0.72], Extrapolation.CLAMP)
        : 1,
      transform: [
        { translateX: translateX.value },
        { rotate: reduceMotion ? '0deg' : `${interpolate(
          translateX.value,
          [-width.value, 0, width.value],
          [-6, 0, 6],
          Extrapolation.CLAMP,
        )}deg` },
      ],
    }));
    const nextStyle = useAnimatedStyle(() => ({
      opacity: interpolate(Math.abs(translateX.value), [0, width.value * 0.4], [0.55, 0.9], Extrapolation.CLAMP),
      transform: [{
        scale: reduceMotion ? 1 : interpolate(
          Math.abs(translateX.value),
          [0, width.value * 0.4],
          [0.965, 0.99],
          Extrapolation.CLAMP,
        ),
      }],
    }));
    const positiveStyle = useAnimatedStyle(() => ({
      opacity: interpolate(-translateX.value, [0, width.value * 0.4], [0, 1], Extrapolation.CLAMP),
    }));
    const negativeStyle = useAnimatedStyle(() => ({
      opacity: interpolate(translateX.value, [0, width.value * 0.4], [0, 1], Extrapolation.CLAMP),
    }));
    const positivePrompt = useAnimatedStyle(() => ({
      opacity: interpolate(-translateX.value / width.value, [0, 0.2, 0.22], [1, 1, 0], Extrapolation.CLAMP),
    }));
    const positiveGuide = useAnimatedStyle(() => ({
      opacity: interpolate(-translateX.value / width.value, [0.18, 0.22, 0.38, 0.42], [0, 1, 1, 0], Extrapolation.CLAMP),
    }));
    const positiveCommit = useAnimatedStyle(() => ({
      opacity: interpolate(-translateX.value / width.value, [0.38, 0.42], [0, 1], Extrapolation.CLAMP),
    }));
    const negativePrompt = useAnimatedStyle(() => ({
      opacity: interpolate(translateX.value / width.value, [0, 0.2, 0.22], [1, 1, 0], Extrapolation.CLAMP),
    }));
    const negativeGuide = useAnimatedStyle(() => ({
      opacity: interpolate(translateX.value / width.value, [0.18, 0.22, 0.38, 0.42], [0, 1, 1, 0], Extrapolation.CLAMP),
    }));
    const negativeCommit = useAnimatedStyle(() => ({
      opacity: interpolate(translateX.value / width.value, [0.38, 0.42], [0, 1], Extrapolation.CLAMP),
    }));

    return (
      <View
        onLayout={(event) => { width.value = event.nativeEvent.layout.width; }}
        style={[styles.deck, compact && styles.deckCompact]}
      >
        <Animated.View style={[styles.affordance, styles.positive, positiveStyle]}>
          <Text style={styles.positiveIcon}>★</Text>
          <Animated.Text style={[styles.affordanceText, positivePrompt]}>{t('teaching.swipe.positive.prompt')}</Animated.Text>
          <Animated.Text style={[styles.affordanceText, styles.layeredText, positiveGuide]}>{t('teaching.swipe.positive.guide')}</Animated.Text>
          <Animated.Text style={[styles.affordanceText, styles.layeredText, positiveCommit]}>✓ {t('teaching.swipe.positive.commit')}</Animated.Text>
          <Text style={styles.affordanceDetail}>{t('teaching.swipe.positive.detail')}</Text>
        </Animated.View>
        <Animated.View style={[styles.affordance, styles.negative, negativeStyle]}>
          <Text style={styles.negativeIcon}>◌</Text>
          <Animated.Text style={[styles.affordanceText, negativePrompt]}>{t('teaching.swipe.negative.prompt')}</Animated.Text>
          <Animated.Text style={[styles.affordanceText, styles.layeredText, negativeGuide]}>{t('teaching.swipe.negative.guide')}</Animated.Text>
          <Animated.Text style={[styles.affordanceText, styles.layeredText, negativeCommit]}>× {t('teaching.swipe.negative.commit')}</Animated.Text>
          <Text style={styles.affordanceDetail}>{t('teaching.swipe.negative.detail')}</Text>
        </Animated.View>
        {nextCard ? (
          <Animated.View pointerEvents="none" style={[styles.cardLayer, styles.nextCard, nextStyle]}>
            <TeachingMessageCard card={nextCard} compact={compact} t={t} />
          </Animated.View>
        ) : null}
        <GestureDetector gesture={pan}>
          <Animated.View
            accessibilityActions={[
              { name: 'activate', label: t('teaching.action.positive') },
              { name: 'decrement', label: t('teaching.action.negative') },
            ]}
            accessibilityHint={t('teaching.card.accessibilityHint')}
            accessibilityLabel={`${card.senderName}. ${card.body}`}
            accessibilityRole="adjustable"
            onAccessibilityAction={(event) => {
              if (event.nativeEvent.actionName === 'activate') animateCommit('positive');
              if (event.nativeEvent.actionName === 'decrement') animateCommit('negative');
            }}
            style={[styles.cardLayer, cardStyle]}
          >
            <TeachingMessageCard card={card} compact={compact} t={t} />
          </Animated.View>
        </GestureDetector>
      </View>
    );
  },
);

const styles = StyleSheet.create({
  deck: { minHeight: 410, justifyContent: 'center' },
  deckCompact: { minHeight: 360 },
  cardLayer: { position: 'absolute', top: 0, right: 0, bottom: 0, left: 0 },
  nextCard: { top: 10 },
  affordance: {
    position: 'absolute', top: 0, right: 0, bottom: 0, left: 0,
    justifyContent: 'center',
    borderRadius: 28,
    paddingHorizontal: 28,
  },
  positive: { alignItems: 'flex-end', backgroundColor: '#123f39' },
  negative: { alignItems: 'flex-start', backgroundColor: '#442934' },
  positiveIcon: { color: colors.success, fontSize: 40 },
  negativeIcon: { color: '#fda4af', fontSize: 42 },
  affordanceText: { maxWidth: '72%', marginTop: 8, color: colors.text, fontSize: 18, fontWeight: '900' },
  layeredText: { position: 'absolute', top: '48%' },
  affordanceDetail: { maxWidth: '72%', marginTop: 8, color: colors.mutedText, fontSize: 12, lineHeight: 18 },
});
