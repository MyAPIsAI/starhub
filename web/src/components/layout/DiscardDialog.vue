<script setup lang="ts">
import { useI18n } from "vue-i18n";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
defineProps<{ open: boolean }>();
const emit = defineEmits<{ answer: [value: boolean] }>();
const { t } = useI18n();
</script>
<template>
  <Dialog
    :open="open"
    @update:open="
      (value) => {
        if (!value) emit('answer', false);
      }
    "
    ><DialogContent
      ><DialogTitle>{{ t("dirtyConfirm") }}</DialogTitle
      ><DialogDescription>{{ t("discardHelp") }}</DialogDescription>
      <div class="form-footer">
        <Button variant="outline" @click="emit('answer', false)">{{
          t("cancel")
        }}</Button
        ><Button variant="destructive" @click="emit('answer', true)">{{
          t("discard")
        }}</Button>
      </div></DialogContent
    ></Dialog
  >
</template>
