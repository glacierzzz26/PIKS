/** 截图上传前的客户端处理：白名单校验 + canvas 降采样（应对后端 5MB 上限）。 */

/** 后端白名单（api_write.go allowedImageType）——不是这几种就明确告知，勿白跑一次 AI。 */
const OK_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"];

/** 粗校验文件类型；返回 null 表示通过，否则返回给用户看的中文原因。 */
export function checkImageType(file: File): string | null {
  const t = (file.type || "").toLowerCase();
  if (t && !OK_TYPES.includes(t)) {
    return "这张图格式不支持，请用截图（PNG / JPEG）。";
  }
  return null;
}

const MAX_EDGE = 2000; // 长边上限：够视觉模型读数，又能把体积压下来

/**
 * 把图片降采样后转成 JPEG（长边 ≤2000）。体积已达标或解码失败时原样返回，
 * 不因压缩失败而阻断上传（后端会如实报错）。
 */
export async function shrinkForVision(file: File): Promise<File> {
  try {
    const bitmap = await createImageBitmap(file);
    const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height));
    if (scale >= 1 && file.size <= 3.5 * 1024 * 1024) {
      bitmap.close?.();
      return file;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    const ctx = canvas.getContext("2d");
    if (!ctx) return file;
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close?.();
    const blob = await new Promise<Blob | null>((res) =>
      canvas.toBlob(res, "image/jpeg", 0.85)
    );
    if (!blob || blob.size >= file.size) return file;
    return new File([blob], file.name.replace(/\.\w+$/, "") + ".jpg", {
      type: "image/jpeg",
    });
  } catch {
    return file; // 解码失败（如浏览器不认的 HEIC）→ 交后端如实报错
  }
}
