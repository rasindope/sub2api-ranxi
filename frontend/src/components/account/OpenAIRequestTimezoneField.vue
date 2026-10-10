<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <label class="input-label">{{ t('admin.accounts.openai.requestTimezone') }}</label>
    <Select
      :model-value="modelValue"
      :options="options"
      searchable
      creatable
      :aria-label="t('admin.accounts.openai.requestTimezone')"
      data-testid="openai-request-timezone-select"
      @update:model-value="selectTimezone"
    />
    <p class="input-hint">{{ t('admin.accounts.openai.requestTimezoneDesc') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'

// 常用的代理出口时区；也可直接输入其他 IANA 时区（后端校验，排除中国大陆及港澳台）。
const COMMON_TIMEZONES = [
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Toronto',
  'America/Vancouver',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Amsterdam',
  'Europe/Berlin',
  'Europe/Paris',
  'Europe/Frankfurt',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Asia/Singapore',
  'Asia/Kuala_Lumpur',
  'Asia/Bangkok',
  'Asia/Kolkata',
  'Asia/Dubai',
  'Australia/Sydney'
]

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ (event: 'update:modelValue', value: string): void }>()
const { t } = useI18n()

const options = computed(() => {
  const values = [...COMMON_TIMEZONES]
  if (props.modelValue && !values.includes(props.modelValue)) values.push(props.modelValue)
  return [
    { value: '', label: t('admin.accounts.openai.requestTimezoneOff') },
    ...values.map(value => ({ value, label: value }))
  ]
})

const selectTimezone = (value: string | number | boolean | null) => {
  emit('update:modelValue', typeof value === 'string' ? value.trim() : '')
}
</script>
