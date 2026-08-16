export async function assertWorkstationDescendantsContained(node, label) {
  const escaped = await node.evaluate((element) => {
    const nodeBounds = element.getBoundingClientRect();

    return Array.from(element.querySelectorAll("*"))
      .map((descendant) => {
        const bounds = descendant.getBoundingClientRect();
        return {
          bottom: bounds.bottom > nodeBounds.bottom,
          descriptor:
            descendant
              .getAttributeNames()
              .find((name) => name.startsWith("data-")) ?? descendant.tagName,
          height: bounds.height,
          left: bounds.left < nodeBounds.left,
          right: bounds.right > nodeBounds.right,
          top: bounds.top < nodeBounds.top,
          width: bounds.width,
        };
      })
      .filter(
        (bounds) =>
          bounds.width > 0 &&
          bounds.height > 0 &&
          (bounds.bottom || bounds.left || bounds.right || bounds.top),
      )
      .map(({ height: _height, width: _width, ...bounds }) => bounds);
  });

  if (escaped.length > 0) {
    throw new Error(
      `${label} content escaped its workstation node: ${JSON.stringify(escaped)}`,
    );
  }
}
