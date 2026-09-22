<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { ref } from "vue";
import type { Field } from "@/features/resources/schema";
defineProps<{
  field: Field;
  modelValue: string | boolean;
  disabled?: boolean;
}>();
defineEmits<{ "update:modelValue": [value: string | boolean] }>();
const { t, te } = useI18n();
const revealed = ref(false);
</script>
<template>
  <div class="form-field" :class="{ wide: field.wide }">
    <label :for="`field-${field.key}`"
      >{{ t(field.key)
      }}<span v-if="field.required" aria-hidden="true"> *</span></label
    >
    <input
      v-if="field.type === 'checkbox'"
      :id="`field-${field.key}`"
      type="checkbox"
      :checked="Boolean(modelValue)"
      :disabled="disabled"
      @change="
        $emit('update:modelValue', ($event.target as HTMLInputElement).checked)
      "
    />
    <select
      v-else-if="field.type === 'select'"
      :id="`field-${field.key}`"
      :value="modelValue"
      :disabled="disabled"
      @change="
        $emit('update:modelValue', ($event.target as HTMLSelectElement).value)
      "
    >
      <option
        v-for="choice in field.choices"
        :key="choice[0]"
        :value="choice[0]"
      >
        {{ te(choice[1]) ? t(choice[1]) : choice[1] }}
      </option>
    </select>
    <textarea
      v-else-if="field.type === 'textarea'"
      :id="`field-${field.key}`"
      :class="{ 'secret-input': field.key === 'key' && !revealed }"
      :value="String(modelValue)"
      :required="field.required"
      :disabled="disabled"
      @input="
        $emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)
      "
    />
    <input
      v-else
      :id="`field-${field.key}`"
      :type="field.type === 'number' ? 'text' : (field.type ?? 'text')"
      :inputmode="field.type === 'number' ? 'numeric' : undefined"
      :value="modelValue"
      :required="field.required"
      :disabled="disabled"
      :autocomplete="field.type === 'password' ? 'new-password' : 'off'"
      :aria-describedby="field.help ? `help-${field.key}` : undefined"
      @input="
        $emit('update:modelValue', ($event.target as HTMLInputElement).value)
      "
    />
    <button
      v-if="field.key === 'key'"
      class="text-button"
      type="button"
      :aria-pressed="revealed"
      @click="revealed = !revealed"
    >
      {{ t(revealed ? "hide" : "show") }}
    </button>
    <p v-if="field.help" :id="`help-${field.key}`" class="help">
      {{ t(field.help) }}
    </p>
  </div>
</template>
