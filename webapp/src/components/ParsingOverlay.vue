<template>
  <div class="parsing-overlay" :hidden="!isParsing" aria-hidden="true">
    <div class="parsing-card" role="status" aria-live="polite">
      <div class="parsing-spinner" aria-hidden="true"></div>
      <div class="parsing-copy">
        <div class="parsing-text">{{ t('parsing.text') }}</div>
        <div v-if="progressLabel" class="parsing-progress">{{ progressLabel }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, inject } from 'vue';
import { useI18n } from 'vue-i18n';
import { fetchAppStatus } from '@/api';

const emit = defineEmits<{
  (e: 'finished'): void;
  (e: 'update:active', value: boolean): void;
}>();

const setParsingActive = inject<((value: boolean) => void) | null>('setParsingActive', null);

const isParsing = ref(false);
const progressPercent = ref<number | null>(null);
const estimatedRemainingSeconds = ref<number | null>(null);
const bodyOverflow = ref<string | null>(null);
const { t } = useI18n({ useScope: 'global' });
const POLL_INTERVAL = 5000;
let timer: number | null = null;
let lastParsing: boolean | null = null;

const progressLabel = computed(() => {
  if (progressPercent.value === null) {
    return '';
  }
  if (estimatedRemainingSeconds.value) {
    const duration = formatDurationSeconds(estimatedRemainingSeconds.value);
    return t('parsing.progressWithRemaining', { value: progressPercent.value, duration });
  }
  return t('parsing.progress', { value: progressPercent.value });
});

function normalizeProgress(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return null;
  }
  return Math.min(100, Math.max(0, Math.round(value)));
}

function normalizeSeconds(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return null;
  }
  const normalized = Math.round(value);
  if (normalized <= 0) {
    return null;
  }
  return normalized;
}

function formatDurationSeconds(seconds: number) {
  const total = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours > 0) {
    return t('overview.durationHoursMinutes', { hours, minutes });
  }
  if (minutes > 0) {
    return t('overview.durationMinutesSeconds', { minutes, seconds: secs });
  }
  return t('overview.durationSeconds', { seconds: secs });
}

function setVisible(value: boolean) {
  isParsing.value = value;
  setParsingActive?.(value);
  emit('update:active', value);
  setBodyScrollLocked(value);
}

function setBodyScrollLocked(locked: boolean) {
  if (typeof document === 'undefined') {
    return;
  }
  const body = document.body;
  if (!body) {
    return;
  }
  if (locked) {
    if (bodyOverflow.value === null) {
      bodyOverflow.value = body.style.overflow;
    }
    body.style.overflow = 'hidden';
    return;
  }
  if (bodyOverflow.value !== null) {
    body.style.overflow = bodyOverflow.value;
    bodyOverflow.value = null;
    return;
  }
  body.style.overflow = '';
}

async function refresh() {
  try {
    const status = await fetchAppStatus();
    const parsing = Boolean(status.log_parsing);
    const wasParsing = lastParsing === true;
    lastParsing = parsing;

    setVisible(parsing);
    progressPercent.value = parsing ? normalizeProgress(status.log_parsing_progress) : null;
    estimatedRemainingSeconds.value = parsing
      ? normalizeSeconds(status.log_parsing_estimated_remaining_seconds)
      : null;

    if (wasParsing && !parsing) {
      emit('finished');
    }

    if (!parsing) {
      stop();
    }
  } catch (error) {
    console.error('获取解析状态失败:', error);
  }
}

function start() {
  if (timer) {
    return;
  }
  timer = window.setInterval(refresh, POLL_INTERVAL);
  refresh();
}

function stop() {
  if (!timer) {
    return;
  }
  window.clearInterval(timer);
  timer = null;
}

function handleVisibility() {
  if (document.hidden) {
    stop();
  } else {
    refresh();
    start();
  }
}

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibility);
  start();
});

onBeforeUnmount(() => {
  stop();
  document.removeEventListener('visibilitychange', handleVisibility);
  setParsingActive?.(false);
  setBodyScrollLocked(false);
});
</script>
