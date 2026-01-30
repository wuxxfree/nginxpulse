<template>
  <div class="mobile-page">
    <section class="mobile-panel">
      <div class="mobile-panel-header">
        <div>
          <div class="section-title">{{ t('app.menu.logs') }}</div>
          <div class="section-sub">{{ t('logs.subtitle') }}</div>
        </div>
        <van-button size="small" type="primary" plain icon="replay" @click="resetAndLoad">
          {{ t('common.refresh') }}
        </van-button>
      </div>
      <van-dropdown-menu>
        <van-dropdown-item v-model="currentWebsiteId" :options="websiteOptions" />
        <van-dropdown-item v-model="sortOrder" :options="sortOrderOptions" />
      </van-dropdown-menu>
    </section>

    <van-empty v-if="!currentWebsiteId && !websitesLoading" :description="t('common.emptyWebsite')" />

    <div v-else class="mobile-page">
      <section class="mobile-panel mobile-filter-card">
        <van-search
          v-model="searchFilter"
          :placeholder="t('logs.searchPlaceholder')"
          shape="round"
          class="mobile-search"
          @search="resetAndLoad"
          @clear="resetAndLoad"
        />
        <van-cell-group class="mobile-filter-group">
          <van-cell :title="t('common.pageview')">
            <template #value>
              <van-switch v-model="pageviewOnly" size="20" />
            </template>
          </van-cell>
        </van-cell-group>
      </section>

      <section class="mobile-panel list-card mobile-log-list">
        <van-list
          v-model:loading="loading"
          :finished="finished"
          :finished-text="t('common.noMore')"
          @load="loadMore"
        >
          <van-cell-group inset>
            <van-cell v-for="item in logs" :key="item.key" :class="['mobile-log-cell', item.statusType]">
              <template #title>
                <div class="mobile-log-item">
                  <div class="mobile-log-title">
                    <span class="method-text">{{ item.method }}</span>
                    <span>{{ item.path }}</span>
                  </div>
                  <div class="mobile-log-meta">{{ item.time }} · {{ item.ip }} · {{ item.location }}</div>
                </div>
              </template>
              <template #value>
                <div class="mobile-tag-group">
                  <van-tag :type="item.statusType">{{ item.statusCode }}</van-tag>
                  <van-tag v-if="item.pageview" plain type="primary">PV</van-tag>
                </div>
              </template>
            </van-cell>
          </van-cell-group>
        </van-list>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { fetchLogs, fetchWebsites } from '@/api';
import type { WebsiteInfo } from '@/api/types';
import { formatLocationLabel } from '@/i18n/mappings';
import { normalizeLocale } from '@/i18n';
import { getUserPreference, saveUserPreference } from '@/utils';

const { t, locale } = useI18n({ useScope: 'global' });

const websites = ref<WebsiteInfo[]>([]);
const websitesLoading = ref(false);
const currentWebsiteId = ref('');
const sortField = ref(getUserPreference('logsSortField', 'timestamp'));
const sortOrder = ref(getUserPreference('logsSortOrder', 'desc'));
const searchFilter = ref('');
const pageviewOnly = ref(false);

const loading = ref(false);
const finished = ref(false);
const page = ref(1);
const pageSize = 20;
const totalPages = ref(0);
const logs = ref<Array<Record<string, any>>>([]);

const currentLocale = computed(() => normalizeLocale(locale.value));

const websiteOptions = computed(() =>
  websites.value.map((site) => ({ text: site.name, value: site.id }))
);

const sortOrderOptions = computed(() => [
  { text: t('logs.sortDesc'), value: 'desc' },
  { text: t('logs.sortAsc'), value: 'asc' },
]);

async function loadWebsites() {
  websitesLoading.value = true;
  try {
    const data = await fetchWebsites();
    websites.value = data || [];
    const saved = getUserPreference('selectedWebsite', '');
    if (saved && websites.value.find((site) => site.id === saved)) {
      currentWebsiteId.value = saved;
    } else if (websites.value.length > 0) {
      currentWebsiteId.value = websites.value[0].id;
    } else {
      currentWebsiteId.value = '';
    }
  } catch (error) {
    console.error('初始化网站失败:', error);
    websites.value = [];
    currentWebsiteId.value = '';
  } finally {
    websitesLoading.value = false;
  }
}

function mapLogItem(log: Record<string, any>, index: number) {
  const time = log.time || t('common.none');
  const ip = log.ip || t('common.none');
  const locationRaw = log.domestic_location || log.global_location || '';
  const location = formatLocationLabel(locationRaw, currentLocale.value, t) || t('common.none');
  const method = log.method || '';
  const url = log.url || '';
  const request = `${method} ${url}`.trim() || t('common.none');
  const path = url || request;
  const statusCode = Number(log.status_code || 0);
  const statusType =
    statusCode >= 500 ? 'danger' : statusCode >= 400 ? 'warning' : statusCode >= 300 ? 'primary' : 'success';
  return {
    key: `${time}-${ip}-${index}`,
    time,
    ip,
    location,
    request,
    method: method || 'GET',
    path,
    statusCode: statusCode || '--',
    statusType,
    pageview: Boolean(log.pageview_flag),
  };
}

async function loadMore() {
  if (loading.value) {
    return;
  }
  loading.value = true;
  if (!currentWebsiteId.value) {
    finished.value = true;
    loading.value = false;
    return;
  }
  try {
    const result = await fetchLogs(
      currentWebsiteId.value,
      page.value,
      pageSize,
      sortField.value,
      sortOrder.value,
      searchFilter.value,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      undefined,
      pageviewOnly.value
    );
    const rawLogs = result.logs || [];
    const mapped = rawLogs.map((log: Record<string, any>, index: number) => mapLogItem(log, index));
    logs.value = logs.value.concat(mapped);
    totalPages.value = result.pagination?.pages || 0;
    if (page.value >= totalPages.value || rawLogs.length === 0) {
      finished.value = true;
    } else {
      page.value += 1;
    }
  } catch (error) {
    console.error('加载日志失败:', error);
    finished.value = true;
  } finally {
    loading.value = false;
  }
}

function resetAndLoad() {
  logs.value = [];
  page.value = 1;
  finished.value = false;
  loading.value = false;
  loadMore();
}


watch(currentWebsiteId, (value) => {
  if (value) {
    saveUserPreference('selectedWebsite', value);
  }
  resetAndLoad();
});

watch([sortOrder, pageviewOnly], () => {
  saveUserPreference('logsSortOrder', sortOrder.value);
  resetAndLoad();
});

onMounted(() => {
  loadWebsites();
});
</script>
