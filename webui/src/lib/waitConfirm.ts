export async function saveWithConfirm<T>(
  put: () => Promise<void>,
  getProfiles: () => Promise<T>,
  onSuccess: (data: T) => void,
  _onError: (e: unknown) => void,
): Promise<T> {
  await put();
  const data = await getProfiles();
  onSuccess(data);
  return data;
}
