/** Pure composition seam for the React hook's child controllers. */
export function composeGraphEditorControllers<
  AddController extends object,
  ConnectionController extends object,
  RemovalController extends object,
>(
  addEntityController: AddController,
  connectionController: ConnectionController,
  removalController: RemovalController,
): { addEntityController: AddController } & ConnectionController &
  RemovalController {
  return {
    addEntityController,
    ...connectionController,
    ...removalController,
  };
}
