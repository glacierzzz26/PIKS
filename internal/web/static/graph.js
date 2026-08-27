/* PIKS 关系图谱 — 原生 SVG 力导向图(无第三方依赖)
 * 功能:局部/全局切换、滚轮缩放、拖拽平移、节点拖拽、点节点看内容面板、双击聚焦、搜索聚焦。
 * 数据:/api/graph(local|global|focus);面板:/api/events/{id}、/api/entities/{id}。
 */
(function () {
  const NS = 'http://www.w3.org/2000/svg';
  const svg = document.getElementById('gSvg');
  const g = document.getElementById('gMain');
  const panel = document.getElementById('gPanel');
  const panelBody = document.getElementById('gPanelBody');
  const panelClose = document.getElementById('gPanelClose');
  const search = document.getElementById('gSearch');
  const btnLocal = document.getElementById('gLocal');
  const btnGlobal = document.getElementById('gGlobal');

  if (!svg) return;

  let W = svg.clientWidth || 820, H = svg.clientHeight || 520;
  let nodes = [], edges = [], nodeById = new Map();
  let nodeEls = new Map(), edgeEls = new Map(), labelEls = new Map();
  let sel = null;           // 选中节点
  let scope = 'local';
  let lastFocus = '';       // 当前 focus 参数(用于聚焦后居中)
  let scale = 1, tx = 0, ty = 0;

  const KIND_COLORS = {
    event: 'var(--accent)',
    company: 'var(--green)',
    industry: 'var(--amber)',
    concept: 'var(--red)',
    topic: 'var(--red)',
    unknown: 'var(--muted)'
  };
  const colorOf = p => KIND_COLORS[p.kind === 'event' ? 'event' : p.type] || KIND_COLORS.unknown;
  const rawIdOf = n => n.url.split('/').pop();

  /* ---------- 视图变换 ---------- */
  function applyViewport() {
    g.setAttribute('transform', 'translate(' + tx + ',' + ty + ') scale(' + scale + ')');
  }
  function zoomAt(sx, sy, factor) {
    const ns = Math.max(0.2, Math.min(3.2, scale * factor));
    const k = ns / scale;
    tx = sx - (sx - tx) * k;
    ty = sy - (sy - ty) * k;
    scale = ns;
    applyViewport();
    render();
  }
  function centerOn(x, y) {
    tx = W / 2 - x * scale;
    ty = H / 2 - y * scale;
    applyViewport();
  }

  /* ---------- 力模拟 ---------- */
  const REP = 2000, SPR = 0.02, DIST = 132, GRAV = 0.006, DAMP = 0.84;
  let running = false, alpha = 1;

  function step() {
    const n = nodes.length;
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const a = nodes[i], b = nodes[j];
        let dx = a.x - b.x, dy = a.y - b.y;
        let d2 = dx * dx + dy * dy;
        if (d2 < 1) { dx = Math.random() - 0.5; dy = Math.random() - 0.5; d2 = 1; }
        const d = Math.sqrt(d2);
        const f = REP / (d2 + 4) * alpha;
        const fx = (dx / d) * f, fy = (dy / d) * f;
        a.vx += fx; a.vy += fy; b.vx -= fx; b.vy -= fy;
      }
    }
    for (const e of edges) {
      const s = nodeById.get(e.source), t = nodeById.get(e.target);
      if (!s || !t) continue;
      const dx = t.x - s.x, dy = t.y - s.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 1;
      const f = (d - DIST) * SPR * alpha;
      const fx = (dx / d) * f, fy = (dy / d) * f;
      s.vx += fx; s.vy += fy; t.vx -= fx; t.vy -= fy;
    }
    const cx = W / 2, cy = H / 2;
    for (const p of nodes) {
      p.vx += (cx - p.x) * GRAV * alpha;
      p.vy += (cy - p.y) * GRAV * alpha;
      p.vx *= DAMP; p.vy *= DAMP;
      p.x += p.vx; p.y += p.vy;
    }
  }
  function loop() {
    if (!running) return;
    step();
    alpha *= 0.985;
    render();
    if (alpha < 0.03) { running = false; alpha = 0; return; } // 收敛即停,不再缓慢漂移
    requestAnimationFrame(loop);
  }
  function restart() { alpha = 1; if (!running) { running = true; requestAnimationFrame(loop); } }

  /* ---------- 场景 ---------- */
  function buildScene() {
    g.textContent = '';
    nodeEls.clear(); edgeEls.clear(); labelEls.clear();
    for (const e of edges) {
      const line = document.createElementNS(NS, 'line');
      line.setAttribute('class', 'g-edge');
      g.appendChild(line);
      edgeEls.set(e, line);
    }
    for (const p of nodes) {
      const c = document.createElementNS(NS, 'circle');
      c.setAttribute('class', 'g-node');
      c.setAttribute('r', p.kind === 'event' ? 6.5 : 5.5);
      c.style.fill = colorOf(p);
      c.style.stroke = 'var(--card)';
      c.style.strokeWidth = '1.6';
      g.appendChild(c);
      nodeEls.set(p, c);
      const t = document.createElementNS(NS, 'text');
      t.setAttribute('class', 'g-label');
      t.textContent = p.label.length > 16 ? p.label.slice(0, 15) + '…' : p.label;
      g.appendChild(t);
      labelEls.set(p, t);
    }
  }
  function render() {
    applyViewport();
    for (const [e, el] of edgeEls) {
      const s = nodeById.get(e.source), t = nodeById.get(e.target);
      if (!s || !t) { el.setAttribute('visibility', 'hidden'); continue; }
      el.setAttribute('visibility', 'visible');
      el.setAttribute('x1', s.x); el.setAttribute('y1', s.y);
      el.setAttribute('x2', t.x); el.setAttribute('y2', t.y);
      if (sel && (e.source === sel.id || e.target === sel.id)) {
        el.style.stroke = 'var(--accent)';
        el.style.strokeWidth = '2';
        el.style.opacity = '1';
      } else {
        el.style.stroke = '';
        el.style.strokeWidth = '1';
        el.style.opacity = '.65';
      }
    }
    const showLabel = scale >= 0.62;
    for (const [p, el] of nodeEls) {
      el.setAttribute('cx', p.x); el.setAttribute('cy', p.y);
      el.style.opacity = sel && sel.id !== p.id ? 0.28 : 1;
    }
    for (const [p, el] of labelEls) {
      el.setAttribute('x', p.x + 9); el.setAttribute('y', p.y + 3.5);
      el.setAttribute('visibility', showLabel ? 'visible' : 'hidden');
    }
  }

  /* ---------- 加载 ---------- */
  async function load(url, focus) {
    let res;
    try {
      res = await fetch(url);
    } catch (_) { return; }
    const data = await res.json();
    if (data.error) return;
    nodes = data.nodes || []; edges = data.edges || [];
    lastFocus = focus || '';
    nodeById.clear();
    nodes.forEach(p => nodeById.set(p.id, p));
    const cx = W / 2, cy = H / 2;
    nodes.forEach((p, i) => {
      const a = (i / Math.max(1, nodes.length)) * Math.PI * 2;
      const r = Math.min(W, H) * 0.34 * (0.5 + (i % 6) / 6);
      p.x = cx + Math.cos(a) * r;
      p.y = cy + Math.sin(a) * r;
      p.vx = (Math.random() - 0.5); p.vy = (Math.random() - 0.5);
    });
    buildScene();
    if (lastFocus) {
      const fid = lastFocus.replace('event:', 'e:').replace('entity:', 'n:');
      const f = nodeById.get(fid);
      if (f) centerOn(f.x, f.y);
    } else { scale = 1; tx = 0; ty = 0; applyViewport(); }
    sel = null;
    render();
    restart();
  }
  function loadLocal() { load('/api/graph?limit=40', ''); }
  function loadGlobal() { load('/api/graph?scope=global', ''); }
  function loadFocus(node) {
    const focus = node.kind + ':' + rawIdOf(node);
    load('/api/graph?focus=' + encodeURIComponent(focus), focus);
  }

  /* ---------- 面板 ---------- */
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }
  function showPanel(html) { panelBody.innerHTML = html; panel.classList.remove('hidden'); }
  function hidePanel() { panel.classList.add('hidden'); sel = null; render(); }
  function openNode(node) {
    sel = node; render();
    const raw = rawIdOf(node);
    const kind = node.kind;
    fetch('/api/' + kind + 's/' + raw).then(r => r.json()).then(d => {
      if (d.error) { showPanel('<p class="empty">' + esc(d.error) + '</p>'); return; }
      showPanel(kind === 'event' ? panelEvent(d, node) : panelEntity(d, node));
    });
  }
  function chip(label, cls) { return '<span class="chip ' + cls + '">' + esc(label) + '</span>'; }
  // zh 后台英文枚举 → "中文(英文)",未收录原样返回。
  function zh(v) {
    const m = {company:'公司',earnings:'业绩',industry:'行业',macro:'宏观',policy:'政策',tech:'科技',
      concept:'概念',topic:'主题',active:'活跃',extracted:'已抽取',merged:'已合并',
      Strong:'强势',Neutral:'中性',Weak:'弱势',limit_up:'涨停数',limit_down:'跌停数',breadth_ratio:'涨跌比',
      broken_rate:'炸板率',max_board:'最高连板',strong_yesterday:'昨日强势',industry_count:'涨停行业数'};
    return m[v] ? m[v] + '(' + v + ')' : v;
  }
  function entChip(name, type, id) {
    return '<a class="chip entity" href="javascript:;" data-open="entity" data-id="' + esc(id) + '" title="点此看实体">' +
      esc(name) + '<em>' + esc(type) + '</em></a>';
  }
  function panelEvent(d, node) {
    const facts = (d.facts || []).map(f => '<li>' + esc(f) + '</li>').join('');
    const aff = (d.affected || []).map(a => a.Linked
      ? entChip(a.EntityName, a.EntityType, a.EntityID)
      : chip(a.Word, 'dim')).join('');
    const evs = (d.evidence || []).map(e => '<li>' + (e.URL ? '<a href="' + esc(e.URL) + '" target="_blank" rel="noopener">' +
      esc(e.Claim) + '</a>' : esc(e.Claim)) + '</li>').join('');
    return '<div class="meta"><span class="chip">' + zh(d.event_type) + '</span>' +
      chip(zh(d.status), 'status') + chip(d.date, 'date') +
      '<span class="chip">置信 ' + Number(d.confidence).toFixed(2) + '</span></div>' +
      '<h3>' + esc(d.title) + '</h3>' +
      (d.summary ? '<p class="kpi" style="margin:6px 0">' + esc(d.summary) + '</p>' : '') +
      '<p class="linkrow"><a href="/events/' + esc(d.id) + '">打开完整事件卡 →</a></p>' +
      (facts ? '<h4>事实</h4><ul>' + facts + '</ul>' : '') +
      (aff ? '<h4>影响</h4><div class="chips">' + aff + '</div>' : '') +
      (evs ? '<h4>证据</h4><ul>' + evs + '</ul>' : '');
  }
  function panelEntity(d, node) {
    const evs = (d.events || []).map(e => '<li><a href="/events/' + esc(e.ID) + '">' + esc(e.Title) + '</a></li>').join('');
    const rels = (d.industries || []).concat(d.companies || []).map(r =>
      entChip(r.Name, r.Type === 'industry' ? '行业' : '公司', r.ID)).join('');
    const zt = (d.zt_dates || []).map(z => chip(z + ' 涨停', 'zt')).join('');
    return '<div class="meta">' + chip(zh(d.type), '') + chip(zh(d.status), 'status') +
      (d.code ? chip(d.code, 'code') : '') + '</div>' +
      '<h3>' + esc(d.name) + '</h3>' +
      (d.description ? '<p class="kpi" style="margin:6px 0">' + esc(d.description) + '</p>' : '') +
      '<p class="linkrow"><a href="/entities/' + esc(d.id) + '">打开完整实体卡 →</a></p>' +
      (evs ? '<h4>相关事件</h4><ul>' + evs + '</ul>' : '') +
      (rels ? '<h4>相关实体</h4><div class="chips">' + rels + '</div>' : '') +
      (zt ? '<h4>涨停记录</h4><div class="chips">' + zt + '</div>' : '');
  }

  /* 面板内打开其他实体(委托) */
  panelBody.addEventListener('click', e => {
    const el = e.target.closest('[data-open]');
    if (!el) return;
    e.preventDefault();
    const id = el.getAttribute('data-id');
    const node = nodeById.get('n:' + id);
    if (node) { openNode(node); } else { fetch('/api/entities/' + id).then(r => r.json()).then(d => { showPanel(panelEntity(d, null)); }); }
  });
  panelClose.addEventListener('click', hidePanel);

  /* ---------- 交互 ---------- */
  // 缩放
  svg.addEventListener('wheel', e => {
    e.preventDefault();
    zoomAt(e.offsetX, e.offsetY, e.deltaY < 0 ? 1.15 : 0.87);
  }, { passive: false });

  // 平移 / 节点拖拽
  let drag = null, pan = null, downAt = null;
  svg.addEventListener('mousedown', e => {
    downAt = { x: e.clientX, y: e.clientY };
    const t = e.target;
    if (t.classList && t.classList.contains('g-node')) {
      const p = nodes.find(n => nodeEls.get(n) === t);
      if (p) { drag = p; sel = p; render(); }
    } else {
      pan = { x: e.clientX, y: e.clientY };
    }
    svg.classList.add('dragging');
  });
  svg.addEventListener('mousemove', e => {
    if (drag) {
      const wx = (e.clientX - svg.getBoundingClientRect().left - tx) / scale;
      const wy = (e.clientY - svg.getBoundingClientRect().top - ty) / scale;
      drag.x = wx; drag.y = wy;
      drag.vx = 0; drag.vy = 0;
      render(); restart();
    } else if (pan) {
      tx += e.clientX - pan.x;
      ty += e.clientY - pan.y;
      pan = { x: e.clientX, y: e.clientY };
      applyViewport(); render();
    }
  });
  svg.addEventListener('mouseup', e => {
    const moved = downAt && Math.hypot(e.clientX - downAt.x, e.clientY - downAt.y) > 5;
    if (drag && !moved) openNode(drag);
    if (!drag && !moved) hidePanel();
    drag = null; pan = null; downAt = null;
    svg.classList.remove('dragging');
  });
  svg.addEventListener('dblclick', e => {
    const t = e.target;
    if (t.classList && t.classList.contains('g-node')) {
      const p = nodes.find(n => nodeEls.get(n) === t);
      if (p) loadFocus(p);
    }
  });

  // 工具栏
  const btnZoomIn = document.getElementById('gZoomIn');
  const btnZoomOut = document.getElementById('gZoomOut');
  const btnReset = document.getElementById('gReset');
  btnZoomIn.addEventListener('click', () => zoomAt(W / 2, H / 2, 1.25));
  btnZoomOut.addEventListener('click', () => zoomAt(W / 2, H / 2, 0.8));
  btnReset.addEventListener('click', () => { scale = 1; tx = 0; ty = 0; applyViewport(); render(); });

  function setScope(s) {
    scope = s;
    btnLocal.classList.toggle('active', s === 'local');
    btnGlobal.classList.toggle('active', s === 'global');
    s === 'local' ? loadLocal() : loadGlobal();
  }
  btnLocal.addEventListener('click', () => setScope('local'));
  btnGlobal.addEventListener('click', () => setScope('global'));

  // 全屏:让图谱容器独占视口,退出/ESC 自动恢复
  const btnFs = document.getElementById('gFullscreen');
  const stageEl = svg.parentElement;
  btnFs.addEventListener('click', () => {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      stageEl.requestFullscreen().catch(() => {});
    }
  });
  document.addEventListener('fullscreenchange', resizeStage);

  // 搜索聚焦
  search.addEventListener('keydown', e => {
    if (e.key !== 'Enter') return;
    const q = search.value.trim().toLowerCase();
    if (!q) return;
    const hit = nodes.find(p => p.label.toLowerCase().includes(q));
    if (!hit) { showPanel('<p class="empty">未找到 "' + esc(search.value) + '"</p>'); return; }
    loadFocus(hit);
  });

  // 尺寸变化(窗口/全屏):保持世界坐标中心不动,内容不跳不漂
  function resizeStage() {
    const cx = (W / 2 - tx) / scale;
    const cy = (H / 2 - ty) / scale;
    W = svg.clientWidth || 820;
    H = svg.clientHeight || 520;
    tx = W / 2 - cx * scale;
    ty = H / 2 - cy * scale;
    applyViewport();
    render();
  }
  window.addEventListener('resize', resizeStage);

  // 启动
  loadLocal();
})();
