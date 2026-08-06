/**
 * Restyle Graphify’s dark HTML chrome to Superopen’s light shell:
 * white stage + subtle grid (like the session map), light sidebar, richer NODE panel.
 */
export function applyLightTheme(html: string): string {
  let out = html;

  // Page / sidebar chrome (string swaps on Graphify’s inline CSS)
  const swaps: [string, string][] = [
    ["background: #0f0f1a", "background: #ffffff"],
    ["color: #e0e0e0", "color: #171717"],
    ["background: #1a1a2e", "background: #ffffff"],
    ["border-left: 1px solid #2a2a4e", "border-left: 1px solid #e5e5e5"],
    ["border-bottom: 1px solid #2a2a4e", "border-bottom: 1px solid #e5e5e5"],
    ["border-top: 1px solid #2a2a4e", "border-top: 1px solid #e5e5e5"],
    ["border: 1px solid #3a3a5e", "border: 1px solid #d4d4d4"],
    ["border-color: #4E79A7", "border-color: #f97316"],
    ["background: #2a2a4e", "background: #f5f5f5"],
    ["color: #aaa", "color: #737373"],
    ["color: #ccc", "color: #525252"],
    ["color: #555", "color: #a3a3a3"],
    ["color: #666", "color: #a3a3a3"],
    ["border-left: 3px solid #333", "border-left: 3px solid #d4d4d4"],
    [
      "background: #4E79A7; border-color: #4E79A7",
      "background: #f97316; border-color: #f97316",
    ],
  ];
  for (const [from, to] of swaps) {
    out = out.split(from).join(to);
  }

  // Node labels readable on white
  out = out
    .split('"font": {"size": 12, "color": "#ffffff"}')
    .join('"font": {"size": 12, "color": "#171717"}');

  // Edges: slightly stronger on white
  out = out
    .split('"color": {"opacity": 0.35}')
    .join('"color": {"color": "#a3a3a3", "opacity": 0.55}');
  out = out
    .split('"color": {"opacity": 0.55}')
    .join('"color": {"color": "#737373", "opacity": 0.7}');

  // Richer NODE inspector (Graphify-style: title, tags, meta, outgoing)
  const oldShowInfo = `function showInfo(nodeId) {
  const n = nodesDS.get(nodeId);
  if (!n) return;
  const neighborIds = network.getConnectedNodes(nodeId);
  const neighborItems = neighborIds.map(nid => {
    const nb = nodesDS.get(nid);
    const color = nb ? nb.color.background : '#555';
    return \`<span class="neighbor-link" style="border-left-color:\${esc(color)}" data-nid="\${esc(nid)}">\${esc(nb ? nb.label : nid)}</span>\`;
  }).join('');
  document.getElementById('info-content').innerHTML = \`
    <div class="field"><b>\${esc(n.label)}</b></div>
    <div class="field">Type: \${esc(n._file_type || 'unknown')}</div>
    <div class="field">Community: \${esc(n._community_name)}</div>
    <div class="field">Source: \${esc(n._source_file || '-')}</div>
    <div class="field">Degree: \${n._degree}</div>
    \${neighborIds.length ? \`<div class="field" style="margin-top:8px;color:#aaa;font-size:11px">Neighbors (\${neighborIds.length})</div><div id="neighbors-list">\${neighborItems}</div>\` : ''}
  \`;
}`;

  const newShowInfo = `function showInfo(nodeId) {
  const n = nodesDS.get(nodeId);
  if (!n) return;
  const neighborIds = network.getConnectedNodes(nodeId);
  const connectedEdges = network.getConnectedEdges(nodeId);
  let incoming = 0, outgoing = 0;
  for (const eid of connectedEdges) {
    const e = edgesDS.get(eid);
    if (!e) continue;
    if (e.to === nodeId) incoming++;
    if (e.from === nodeId) outgoing++;
  }
  const kind = String(n._file_type || 'unknown').toUpperCase();
  const communityColor = (n.color && n.color.background) ? n.color.background : '#f97316';
  const outItems = neighborIds.map(nid => {
    const nb = nodesDS.get(nid);
    const color = nb && nb.color ? nb.color.background : '#a3a3a3';
    const nbKind = nb && nb._file_type ? String(nb._file_type).toUpperCase() : '';
    return '<button type="button" class="neighbor-link" style="border-left-color:' + esc(color) + '" data-nid="' + esc(nid) + '">' +
      '<span class="neighbor-name">' + esc(nb ? nb.label : nid) + '</span>' +
      (nbKind ? '<span class="neighbor-kind">' + esc(nbKind) + '</span>' : '') +
      '</button>';
  }).join('');
  document.getElementById('info-content').innerHTML =
    '<div class="node-head">' +
      '<div class="node-title">' + esc(n.label) + '</div>' +
      '<div class="node-tags">' +
        '<span class="tag"><i class="tag-dot" style="background:' + esc(communityColor) + '"></i>' + esc(kind) + '</span>' +
        (n._community_name ? '<span class="tag muted"><i class="tag-dot" style="background:' + esc(communityColor) + '"></i>COMMUNITY</span>' : '') +
      '</div>' +
    '</div>' +
    '<div class="meta-grid">' +
      '<div class="meta-row"><span>path</span><b title="' + esc(n._source_file || '') + '">' + esc(n._source_file || '-') + '</b></div>' +
      '<div class="meta-row"><span>community</span><b><i class="tag-dot" style="background:' + esc(communityColor) + '"></i>' + esc(n._community_name || '-') + '</b></div>' +
      '<div class="meta-row"><span>degree</span><b>' + esc(String(n._degree ?? neighborIds.length)) +
        ' · ' + incoming + ' in · ' + outgoing + ' out</b></div>' +
    '</div>' +
    (neighborIds.length
      ? '<div class="section-label">OUTGOING ' + neighborIds.length + '</div><div id="neighbors-list">' + outItems + '</div>'
      : '<div class="empty" style="margin-top:10px">No connected neighbors</div>');
}`;

  if (out.includes(oldShowInfo)) {
    out = out.replace(oldShowInfo, newShowInfo);
  } else {
    // Fallback: replace by function start through next function/network handler
    out = out.replace(
      /function showInfo\(nodeId\) \{[\s\S]*?\n\}\n/,
      newShowInfo + "\n"
    );
  }

  // Sidebar heading casing to match Graphify
  out = out.replace(
    "<h3>Node Info</h3>",
    "<h3>Node</h3>"
  );
  out = out.replace(
    "<h3>Communities</h3>",
    '<h3>Communities <span id="community-total"></span></h3>'
  );

  // Communities come from Graphify's LEGEND - do not synthesize them here.

  const override = `<style id="so-light">
  html, body {
    background: #ffffff !important;
    color: #171717 !important;
  }
  /* White stage + map-style grid behind the vis canvas */
  #graph {
    position: relative;
    background-color: #ffffff !important;
    background-image:
      linear-gradient(#ececec 1px, transparent 1px),
      linear-gradient(90deg, #ececec 1px, transparent 1px) !important;
    background-size: 28px 28px !important;
    background-position: -1px -1px !important;
  }
  #graph canvas {
    background: transparent !important;
  }
  #sidebar {
    width: 300px !important;
    background: #ffffff !important;
    border-left: 1px solid #e5e5e5 !important;
  }
  #search {
    background: #fafafa !important;
    border-color: #d4d4d4 !important;
    color: #171717 !important;
  }
  #search:focus { border-color: #f97316 !important; }
  .search-item:hover, .legend-item:hover { background: #f5f5f5 !important; }
  #info-panel, #search-wrap, #search-results, #stats {
    border-color: #e5e5e5 !important;
  }
  #info-panel {
    min-height: 180px !important;
    max-height: 48vh;
    overflow-y: auto;
  }
  #info-panel h3, #legend-wrap h3 {
    color: #737373 !important;
    font-size: 11px !important;
    letter-spacing: 0.08em !important;
  }
  #community-total { color: #a3a3a3; font-weight: 500; margin-left: 6px; }
  #info-content { color: #525252 !important; }
  #info-content .empty { color: #a3a3a3 !important; }
  .node-head { margin-bottom: 12px; }
  .node-title {
    font-size: 16px;
    font-weight: 650;
    color: #171717;
    line-height: 1.3;
    word-break: break-word;
    margin-bottom: 8px;
  }
  .node-tags { display: flex; flex-wrap: wrap; gap: 6px; }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 8px;
    border-radius: 999px;
    border: 1px solid #e5e5e5;
    background: #fafafa;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: #404040;
  }
  .tag.muted { color: #737373; font-weight: 500; }
  .tag-dot, .meta-row b .tag-dot {
    display: inline-block;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .meta-grid {
    display: grid;
    gap: 6px;
    padding: 10px 0;
    border-top: 1px solid #f0f0f0;
    border-bottom: 1px solid #f0f0f0;
  }
  .meta-row {
    display: grid;
    grid-template-columns: 84px minmax(0, 1fr);
    gap: 8px;
    font-size: 12px;
    align-items: baseline;
  }
  .meta-row > span { color: #a3a3a3; text-transform: lowercase; }
  .meta-row b {
    color: #171717;
    font-weight: 550;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .section-label {
    margin-top: 12px;
    margin-bottom: 6px;
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: #737373;
    font-weight: 600;
  }
  .neighbor-link {
    display: flex !important;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    text-align: left;
    background: transparent;
    border: none;
    border-left: 3px solid #d4d4d4;
    padding: 5px 8px !important;
    margin: 2px 0;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
    color: #171717;
  }
  .neighbor-link:hover { background: #f5f5f5 !important; }
  .neighbor-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .neighbor-kind {
    flex-shrink: 0;
    font-size: 10px;
    letter-spacing: 0.04em;
    color: #a3a3a3;
  }
  .legend-count, #stats { color: #a3a3a3 !important; }
  #stats { border-top-color: #e5e5e5 !important; }
  .legend-cb:checked, #select-all-cb:checked,
  #select-all-cb:indeterminate {
    background: #f97316 !important;
    border-color: #f97316 !important;
  }
  #legend-controls label { color: #737373 !important; }
  #legend-controls label:hover { color: #171717 !important; }
  #legend {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-height: 0;
  }
  .legend-item {
    color: #171717 !important;
    font-size: 12px !important;
  }
  .legend-label {
    color: #171717 !important;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .legend-count { color: #a3a3a3 !important; }
  .legend-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }
</style>
<script id="so-light-boot">
(function () {
  function paintGraphStage() {
    var g = document.getElementById('graph');
    if (!g) return;
    g.style.backgroundColor = '#ffffff';
    g.style.backgroundImage =
      'linear-gradient(#ececec 1px, transparent 1px), linear-gradient(90deg, #ececec 1px, transparent 1px)';
    g.style.backgroundSize = '28px 28px';
    var canvases = g.querySelectorAll('canvas');
    canvases.forEach(function (c) {
      c.style.background = 'transparent';
    });
  }
  function fillCommunityTotal() {
    var el = document.getElementById('community-total');
    var legend = document.getElementById('legend');
    if (!el || !legend) return;
    var n = legend.querySelectorAll('.legend-item').length;
    if (n) el.textContent = String(n);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () {
      paintGraphStage();
      setTimeout(fillCommunityTotal, 0);
      setTimeout(paintGraphStage, 50);
      setTimeout(paintGraphStage, 500);
    });
  } else {
    paintGraphStage();
    setTimeout(fillCommunityTotal, 0);
    setTimeout(paintGraphStage, 50);
  }
  // Re-assert after vis mounts its canvas
  var obs = new MutationObserver(paintGraphStage);
  document.addEventListener('DOMContentLoaded', function () {
    var g = document.getElementById('graph');
    if (g) obs.observe(g, { childList: true, subtree: true });
  });
})();
</script>`;

  if (out.includes("</head>")) {
    out = out.replace("</head>", `${override}</head>`);
  } else {
    out = override + out;
  }

  return out;
}

/**
 * Restyle Graphify’s bluish navy chrome to Superopen dark shell:
 * pure black stage + neutral sidebar (matches UI dark theme).
 */
export function applyDarkTheme(html: string): string {
  let out = html;

  const swaps: [string, string][] = [
    ["background: #0f0f1a", "background: #000000"],
    ["background: #1a1a2e", "background: #0a0a0a"],
    ["border-left: 1px solid #2a2a4e", "border-left: 1px solid #262626"],
    ["border-bottom: 1px solid #2a2a4e", "border-bottom: 1px solid #262626"],
    ["border-top: 1px solid #2a2a4e", "border-top: 1px solid #262626"],
    ["border: 1px solid #3a3a5e", "border: 1px solid #404040"],
    ["background: #2a2a4e", "background: #171717"],
    ["border-left: 3px solid #333", "border-left: 3px solid #404040"],
  ];
  for (const [from, to] of swaps) {
    out = out.split(from).join(to);
  }

  const override = `<style id="so-dark">
  html, body {
    background: #000000 !important;
    color: #e5e5e5 !important;
  }
  #graph {
    position: relative;
    background-color: #000000 !important;
    background-image:
      linear-gradient(#1a1a1a 1px, transparent 1px),
      linear-gradient(90deg, #1a1a1a 1px, transparent 1px) !important;
    background-size: 28px 28px !important;
    background-position: -1px -1px !important;
  }
  #graph canvas {
    background: transparent !important;
  }
  #sidebar {
    width: 300px !important;
    background: #0a0a0a !important;
    border-left: 1px solid #262626 !important;
    color: #e5e5e5 !important;
  }
  #search {
    background: #111111 !important;
    border-color: #404040 !important;
    color: #fafafa !important;
  }
  #search:focus { border-color: #f97316 !important; }
  .search-item:hover, .legend-item:hover { background: #171717 !important; }
  #info-panel, #search-wrap, #search-results, #stats {
    border-color: #262626 !important;
  }
  #info-panel h3, #legend-wrap h3 {
    color: #a3a3a3 !important;
  }
  #info-content { color: #d4d4d4 !important; }
  #info-content .empty { color: #737373 !important; }
  .node-title { color: #fafafa !important; }
  .tag {
    border-color: #404040 !important;
    background: #171717 !important;
    color: #d4d4d4 !important;
  }
  .meta-grid {
    border-top-color: #262626 !important;
    border-bottom-color: #262626 !important;
  }
  .meta-row > span { color: #737373 !important; }
  .meta-row b { color: #fafafa !important; }
  .neighbor-link {
    color: #e5e5e5 !important;
    border-left-color: #404040 !important;
  }
  .neighbor-link:hover { background: #171717 !important; }
  .legend-item, .legend-label { color: #e5e5e5 !important; }
  .legend-count, #stats { color: #737373 !important; }
  #stats { border-top-color: #262626 !important; }
  #legend-controls label { color: #a3a3a3 !important; }
  #legend-controls label:hover { color: #fafafa !important; }
  /* vis.js edge/node hover tooltip - Graphify default is white */
  div.vis-tooltip,
  .vis-tooltip {
    background: #171717 !important;
    color: #fafafa !important;
    border: 1px solid #404040 !important;
    box-shadow: 0 4px 16px rgb(0 0 0 / 45%) !important;
  }
</style>
<script id="so-dark-boot">
(function () {
  function paintGraphStage() {
    var g = document.getElementById('graph');
    if (!g) return;
    g.style.backgroundColor = '#000000';
    g.style.backgroundImage =
      'linear-gradient(#1a1a1a 1px, transparent 1px), linear-gradient(90deg, #1a1a1a 1px, transparent 1px)';
    g.style.backgroundSize = '28px 28px';
    var canvases = g.querySelectorAll('canvas');
    canvases.forEach(function (c) {
      c.style.background = 'transparent';
    });
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () {
      paintGraphStage();
      setTimeout(paintGraphStage, 50);
      setTimeout(paintGraphStage, 500);
    });
  } else {
    paintGraphStage();
    setTimeout(paintGraphStage, 50);
  }
  var obs = new MutationObserver(paintGraphStage);
  document.addEventListener('DOMContentLoaded', function () {
    var g = document.getElementById('graph');
    if (g) obs.observe(g, { childList: true, subtree: true });
  });
})();
</script>`;

  if (out.includes("</head>")) {
    out = out.replace("</head>", `${override}</head>`);
  } else {
    out = override + out;
  }

  return out;
}
