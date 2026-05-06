export async function evalPath (path) {
  try {
    await import(path);
  } catch (err) {
    console.error("Error evaluating code:", err);
  }
};