<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { use } from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  AriaComponent,
} from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import VChart from "vue-echarts";
import { usePreferredDark, usePreferredReducedMotion } from "@vueuse/core";
import { useUI } from "@/stores/ui";
import type { DailyUsage } from "@/lib/types";
use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  AriaComponent,
  SVGRenderer,
]);
const props = defineProps<{ items: DailyUsage[] }>();
const ui = useUI();
const { t } = useI18n();
const dark = usePreferredDark();
const motion = usePreferredReducedMotion();
const option = computed(() => {
  const text =
    ui.theme === "dark" || (ui.theme === "system" && dark.value)
      ? "#a0aec0"
      : "#667387";
  return {
    animation: motion.value !== "reduce",
    aria: { enabled: true },
    color: ["#149c83", "#d3a04b"],
    tooltip: { trigger: "axis", confine: true },
    grid: { left: 65, right: 24, top: 50, bottom: 40 },
    legend: { top: 0, textStyle: { color: text } },
    textStyle: { color: text },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: props.items.map((i) => i.date.slice(5)),
      axisLine: { lineStyle: { color: text, opacity: 0.15 } },
      axisTick: { show: false },
      axisLabel: { color: text },
    },
    yAxis: {
      type: "value",
      name: ui.unit === "quota" ? "quota" : "USD",
      axisLabel: { color: text },
      splitLine: { lineStyle: { color: text, opacity: 0.12, type: "dashed" } },
    },
    series: ["quota", "unfunded_quota"].map((key, index) => ({
      name: t(index ? "unfunded_quota" : "charged"),
      type: "line",
      smooth: 0.25,
      showSymbol: false,
      lineStyle: { width: 2, type: index ? "dashed" : "solid" },
      areaStyle: index ? undefined : { opacity: 0.07 },
      data: props.items.map(
        (i) =>
          i[key as "quota" | "unfunded_quota"] /
          (ui.unit === "quota" ? 1 : 500000),
      ),
    })),
  };
});
</script>
<template>
  <VChart
    class="dashboard-chart"
    :option="option"
    :init-options="{ renderer: 'svg' }"
    autoresize
  />
</template>
