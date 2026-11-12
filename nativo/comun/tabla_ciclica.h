/* Generado por internal/muestrabin/tabla_test.go: NO EDITAR.
 * Seno y coseno de hora (0-23), día de la semana (0 = domingo) y mes
 * (1-12; la posición 0 no se usa) con los mismos bits que calcula Go en
 * dataset.VectorFeatures, para que muestra.bin salga idéntico desde C. */
#ifndef SACA_TABLA_CICLICA_H
#define SACA_TABLA_CICLICA_H

static const double SACA_SIN_HORA[24] = {0x0p+00, 0x1.0907dc193069p-02, 0x1.fffffffffffffp-02, 0x1.6a09e667f3bccp-01, 0x1.bb67ae8584caap-01, 0x1.ee8dd4748bf15p-01, 0x1p+00, 0x1.ee8dd4748bf15p-01, 0x1.bb67ae8584cacp-01, 0x1.6a09e667f3bcdp-01, 0x1.fffffffffffffp-02, 0x1.0907dc1930695p-02, 0x1.1a62633145cp-53, -0x1.0907dc1930691p-02, -0x1.ffffffffffffcp-02, -0x1.6a09e667f3bc9p-01, -0x1.bb67ae8584ca8p-01, -0x1.ee8dd4748bf15p-01, -0x1p+00, -0x1.ee8dd4748bf16p-01, -0x1.bb67ae8584caap-01, -0x1.6a09e667f3bcep-01, -0x1.0000000000004p-01, -0x1.0907dc193069ep-02};
static const double SACA_COS_HORA[24] = {0x1p+00, 0x1.ee8dd4748bf15p-01, 0x1.bb67ae8584cabp-01, 0x1.6a09e667f3bcdp-01, 0x1.0000000000001p-01, 0x1.0907dc193069p-02, 0x1.1a62633145cp-54, -0x1.0907dc193068ep-02, -0x1.ffffffffffffcp-02, -0x1.6a09e667f3bccp-01, -0x1.bb67ae8584cabp-01, -0x1.ee8dd4748bf14p-01, -0x1p+00, -0x1.ee8dd4748bf15p-01, -0x1.bb67ae8584cacp-01, -0x1.6a09e667f3bdp-01, -0x1.0000000000004p-01, -0x1.0907dc193068ep-02, -0x1.a79394c9e8ap-53, 0x1.0907dc1930688p-02, 0x1p-01, 0x1.6a09e667f3bccp-01, 0x1.bb67ae8584ca8p-01, 0x1.ee8dd4748bf13p-01};
static const double SACA_SIN_DIA[7] = {0x0p+00, 0x1.904c37505de4bp-01, 0x1.f329c0558e96ap-01, 0x1.bc4c04d71abc3p-02, -0x1.bc4c04d71abbfp-02, -0x1.f329c0558e969p-01, -0x1.904c37505de4dp-01};
static const double SACA_COS_DIA[7] = {0x1p+00, 0x1.3f3a0e28bedd2p-01, -0x1.c7b90e302458p-03, -0x1.cd4bca9cb5c7p-01, -0x1.cd4bca9cb5c71p-01, -0x1.c7b90e3024589p-03, 0x1.3f3a0e28beddp-01};
static const double SACA_SIN_MES[13] = {0, 0x1.fffffffffffffp-02, 0x1.bb67ae8584caap-01, 0x1p+00, 0x1.bb67ae8584cacp-01, 0x1.fffffffffffffp-02, 0x1.1a62633145cp-53, -0x1.ffffffffffffcp-02, -0x1.bb67ae8584ca8p-01, -0x1p+00, -0x1.bb67ae8584caap-01, -0x1.0000000000004p-01, -0x1.1a62633145cp-52};
static const double SACA_COS_MES[13] = {0, 0x1.bb67ae8584cabp-01, 0x1.0000000000001p-01, 0x1.1a62633145cp-54, -0x1.ffffffffffffcp-02, -0x1.bb67ae8584cabp-01, -0x1p+00, -0x1.bb67ae8584cacp-01, -0x1.0000000000004p-01, -0x1.a79394c9e8ap-53, 0x1p-01, 0x1.bb67ae8584ca8p-01, 0x1p+00};

#endif
