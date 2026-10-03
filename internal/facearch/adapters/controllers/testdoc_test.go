package controllers_test

// testDoc is a copy of the profile document internal/facearch's
// interpret_test.go uses, so the handler tests run the same interpretation.
// testDoc's numbers are test inputs, not proposed values.
const testDoc = `{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]},
  "measurementsShown": ["eye_width", "fifths", "gone_key"],
  "classifiers": [{
    "key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5,
    "classes": [
      {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
      {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}
    ]
  }],
  "traits": [{
    "key": "noseWidth",
    "rules": [
      {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]},
      {"label": "Narrow", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": "<", "value": 0.9}]}
    ],
    "default": "Average"
  }]
}`
