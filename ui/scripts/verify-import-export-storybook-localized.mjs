export function createLocalizedExportDialogVerifier({
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
}) {
  return async function verifyLocalizedExportDialog(page, dialog, viewport) {
    await expectVisible(
      dialog.getByRole("textbox", { name: "工厂名称" }),
      "Localized factory name input",
    );
    await expectVisible(dialog.getByLabel("封面图片"), "Localized cover image input");
    await expectVisible(
      dialog.getByRole("button", { name: "取消" }),
      "Localized export cancel button",
    );
    await expectVisible(
      dialog.getByRole("button", { name: "导出 PNG" }),
      "Localized export action button",
    );
    await expectVisible(
      dialog.getByText("确认导出不会更改当前仪表板状态"),
      "Localized export helper copy",
    );
    await expectDialogWithinViewport(dialog, viewport, "Localized export");
    await expectNoHorizontalOverflow(
      page,
      `Localized export dialog at ${viewport.label}`,
    );
  };
}

export function createLocalizedImportDialogVerifier({
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
}) {
  return async function verifyLocalizedImportDialog(page, dialog, viewport) {
    await expectVisible(
      dialog.getByRole("img", { name: "Dropped Factory 预览图" }),
      "Localized import preview image",
    );
    await expectVisible(dialog.getByText("factory-import.png"), "Localized dropped file name");
    await expectVisible(
      dialog.getByRole("button", { name: "取消导入" }),
      "Localized import cancel button",
    );
    await expectVisible(
      dialog.getByRole("button", { name: "启用工厂" }),
      "Localized import activate button",
    );
    await expectVisible(
      dialog.getByRole("button", { name: "关闭导入预览" }),
      "Localized import close button",
    );
    await expectDialogWithinViewport(dialog, viewport, "Localized import preview");
    await expectNoHorizontalOverflow(
      page,
      `Localized import preview dialog at ${viewport.label}`,
    );
  };
}
