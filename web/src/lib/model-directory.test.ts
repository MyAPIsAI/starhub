import { describe, expect, it } from "vitest";
import { createDefaultPricing } from "./model-pricing";
import {
  directoryCandidates,
  directoryPrices,
  getDirectoryModel,
} from "../config/model-directory";
import { describeDirectoryRate } from "../components/directory-price";

describe("directory billing consumer", () => {
  it("quotes custom groups only for the explicitly authorized current account", () => {
    const config = createDefaultPricing();
    config.groups.pro.visibility = "custom";
    config.groups.pro.accountIds = ["account-new"];
    for (const id of ["gpt-4.1", "veyra-auto"]) {
      const model = getDirectoryModel(id)!;
      const expected = directoryPrices(
        model,
        "pro",
        createDefaultPricing(),
        "2026-09-27T12:00:00Z",
      );
      const authorized = directoryPrices(
        model,
        "pro",
        config,
        "2026-09-27T12:00:00Z",
        "account-new",
      );
      expect(authorized.available).toBe(true);
      expect(authorized.minAmountNano).toBe(expected.minAmountNano);
      expect(
        directoryCandidates(model, "pro", config, "account-new").length,
      ).toBeGreaterThan(0);
      for (const accountId of ["account-demo", "account-other", ""]) {
        expect(directoryCandidates(model, "pro", config, accountId)).toEqual(
          [],
        );
        expect(
          directoryPrices(
            model,
            "pro",
            config,
            "2026-09-27T12:00:00Z",
            accountId,
          ),
        ).toMatchObject({
          rates: [],
          available: false,
          canEstimate: false,
          minAmountNano: undefined,
        });
      }
    }
  });
  it("limits smart routing to online text models with a client protocol", () => {
    const config = createDefaultPricing();
    const candidates = directoryCandidates(
      getDirectoryModel("veyra-auto")!,
      "pro",
      config,
    );
    expect(candidates.length).toBeGreaterThan(0);
    expect(
      candidates.every(
        (model) =>
          model.capabilities.includes("text") && model.protocols.length,
      ),
    ).toBe(true);
    expect(candidates.some((model) => model.id.startsWith("veyra-demo-"))).toBe(
      false,
    );
    config.groups.pro.enabled = false;
    expect(
      directoryPrices(getDirectoryModel("veyra-auto")!, "pro", config)
        .available,
    ).toBe(false);
  });
  it("quotes image and video units instead of their unused zero token rates", () => {
    const config = createDefaultPricing();
    const image = directoryPrices(
      getDirectoryModel("veyra-demo-image")!,
      "pro",
      config,
    ).rates[0];
    const video = directoryPrices(
      getDirectoryModel("veyra-demo-video")!,
      "pro",
      config,
    ).rates[0];
    expect(image.quote?.amountNano).toBe("32000000");
    expect(video.quote?.amountNano).toBe("64000000");
    expect(describeDirectoryRate(image, "zh").unit).toBe("美元 / 张");
    expect(describeDirectoryRate(video, "zh").unit).toBe("美元 / 秒");
  });
  it("applies the matched time window once to both displayed rate and estimate", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "tokens",
      timeWindows: [
        {
          id: "night",
          label: "夜间",
          timezone: "UTC",
          days: [0, 1, 2, 3, 4, 5, 6],
          start: "00:00",
          end: "08:00",
          multiplier: "0.5",
        },
      ],
    };
    const price = directoryPrices(
      getDirectoryModel("gpt-4.1")!,
      "standard",
      config,
      "2026-09-26T03:00:00Z",
    );
    expect(price.rates[0].input).toBe("0.6");
    expect(price.minEstimate).toBeCloseTo(0.00132);
    expect(describeDirectoryRate(price.rates[0], "zh").schedule).toContain(
      "夜间",
    );
  });
  it("describes changed expression rules and reports failures without a free-price fallback", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression: "requests * 0.1",
    };
    let price = directoryPrices(getDirectoryModel("gpt-4.1")!, "pro", config);
    expect(price.rates[0].quote?.amountNano).toBe("80000000");
    const description = describeDirectoryRate(price.rates[0], "zh");
    expect(description.value).toBe("$0.080000");
    expect(description.mode).toBe("表达式计价");
    expect(description.assumption).toContain("0 张图片、0 秒、1 次请求");
    config.models["gpt-4.1"].rule.expression = "requests / images";
    price = directoryPrices(getDirectoryModel("gpt-4.1")!, "pro", config);
    expect(price.available).toBe(true);
    expect(price.canEstimate).toBe(false);
    expect(price.minAmountNano).toBeUndefined();
    expect(describeDirectoryRate(price.rates[0], "zh").value).toBe(
      "暂不可估算",
    );
    expect(price.rates[0].error).toContain("除数不能为零");
  });
});
